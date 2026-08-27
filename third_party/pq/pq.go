package pq

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

func init() {
	sql.Register("postgres", &Driver{})
}

type Driver struct{}

func (d *Driver) Open(name string) (driver.Conn, error) {
	cfg, err := parseDSN(name)
	if err != nil {
		return nil, err
	}
	return cfg.open()
}

type config struct {
	user     string
	password string
	database string
	host     string
	port     int
}

func parseDSN(dsn string) (*config, error) {
	cfg := &config{host: "localhost", port: 5432}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return nil, err
		}
		cfg.user = u.User.Username()
		cfg.password, _ = u.User.Password()
		cfg.host = u.Hostname()
		if cfg.host == "" {
			cfg.host = "localhost"
		}
		if p := u.Port(); p != "" {
			port, err := strconv.Atoi(p)
			if err != nil {
				return nil, err
			}
			cfg.port = port
		}
		cfg.database = strings.TrimPrefix(u.Path, "/")
		if cfg.database == "" {
			cfg.database = cfg.user
		}
		q := u.Query()
		if v := q.Get("user"); v != "" {
			cfg.user = v
		}
		if v := q.Get("password"); v != "" {
			cfg.password = v
		}
		if v := q.Get("dbname"); v != "" {
			cfg.database = v
		}
		if v := q.Get("host"); v != "" {
			cfg.host = v
		}
		if v := q.Get("port"); v != "" {
			port, err := strconv.Atoi(v)
			if err != nil {
				return nil, err
			}
			cfg.port = port
		}
		return cfg, nil
	}

	fields := splitDSNFields(dsn)
	for _, field := range fields {
		if field == "" {
			continue
		}
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			return nil, fmt.Errorf("invalid dsn field %q", field)
		}
		switch key {
		case "user":
			cfg.user = value
		case "password":
			cfg.password = value
		case "dbname", "database":
			cfg.database = value
		case "host":
			cfg.host = value
		case "port":
			port, err := strconv.Atoi(value)
			if err != nil {
				return nil, err
			}
			cfg.port = port
		}
	}
	if cfg.database == "" {
		cfg.database = cfg.user
	}
	if cfg.user == "" {
		cfg.user = cfg.database
	}
	return cfg, nil
}

func splitDSNFields(dsn string) []string {
	var out []string
	var cur strings.Builder
	inQuotes := false
	escaped := false
	for _, r := range dsn {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\' && inQuotes:
			escaped = true
		case r == '\'':
			inQuotes = !inQuotes
		case r == ' ' && !inQuotes:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

type conn struct {
	mu       sync.Mutex
	cfg      *config
	c        net.Conn
	br       *bufio.Reader
	scram    *scramState
	closed   bool
	txActive bool
}

func (cfg *config) open() (*conn, error) {
	var network, address string
	if strings.HasPrefix(cfg.host, "/") {
		network = "unix"
		address = fmt.Sprintf("%s/.s.PGSQL.%d", cfg.host, cfg.port)
	} else {
		network = "tcp"
		address = net.JoinHostPort(cfg.host, strconv.Itoa(cfg.port))
	}
	c, err := net.DialTimeout(network, address, 5*time.Second)
	if err != nil {
		return nil, err
	}
	conn := &conn{cfg: cfg, c: c, br: bufio.NewReader(c)}
	if err := conn.startup(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return conn, nil
}

func (c *conn) startup() error {
	params := map[string]string{
		"user":            c.cfg.user,
		"database":        c.cfg.database,
		"client_encoding": "UTF8",
	}
	if err := c.writeStartup(params); err != nil {
		return err
	}
	return c.readUntilReady()
}

func (c *conn) writeStartup(params map[string]string) error {
	var payload []byte
	payload = binary.BigEndian.AppendUint32(payload, 196608)
	for k, v := range params {
		payload = append(payload, k...)
		payload = append(payload, 0)
		payload = append(payload, v...)
		payload = append(payload, 0)
	}
	payload = append(payload, 0)
	// The PostgreSQL startup packet has no leading message-type byte; it is
	// framed only by a 4-byte length prefix covering the length field itself.
	if c.c == nil {
		return io.ErrClosedPipe
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)+4))
	if _, err := c.c.Write(header[:]); err != nil {
		return err
	}
	_, err := c.c.Write(payload)
	return err
}

func (c *conn) writeMessage(typ byte, payload []byte) error {
	if c.c == nil {
		return io.ErrClosedPipe
	}
	var header [5]byte
	header[0] = typ
	binary.BigEndian.PutUint32(header[1:], uint32(len(payload)+4))
	if _, err := c.c.Write(header[:]); err != nil {
		return err
	}
	_, err := c.c.Write(payload)
	return err
}

func (c *conn) readMessage() (byte, []byte, error) {
	typ, err := c.br.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	var lenBuf [4]byte
	if _, err := io.ReadFull(c.br, lenBuf[:]); err != nil {
		return 0, nil, err
	}
	length := binary.BigEndian.Uint32(lenBuf[:])
	if length < 4 {
		return 0, nil, errors.New("invalid message length")
	}
	payload := make([]byte, length-4)
	if _, err := io.ReadFull(c.br, payload); err != nil {
		return 0, nil, err
	}
	return typ, payload, nil
}

func (c *conn) readUntilReady() error {
	for {
		typ, payload, err := c.readMessage()
		if err != nil {
			return err
		}
		switch typ {
		case 'R':
			if err := c.handleAuth(payload); err != nil {
				return err
			}
		case 'S', 'K', 'N':
			continue
		case 'Z':
			return nil
		case 'E':
			return parseError(payload)
		default:
			continue
		}
	}
}

func (c *conn) handleAuth(payload []byte) error {
	if len(payload) < 4 {
		return errors.New("invalid auth message")
	}
	code := int32(binary.BigEndian.Uint32(payload[:4]))
	switch code {
	case 0:
		return nil
	case 3:
		return c.writePassword(c.cfg.password)
	case 5:
		if len(payload) < 8 {
			return errors.New("invalid md5 auth message")
		}
		salt := payload[4:8]
		return c.writePassword(md5Password(c.cfg.password, c.cfg.user, salt))
	case 10:
		return c.handleSASL(payload[4:])
	case 11:
		return c.handleSASLContinue(payload[4:])
	case 12:
		return c.handleSASLFinal(payload[4:])
	default:
		return fmt.Errorf("unsupported postgres auth request %d", code)
	}
}

func (c *conn) writePassword(password string) error {
	return c.writeMessage('p', append([]byte(password), 0))
}

func md5Password(password, user string, salt []byte) string {
	h1 := md5.Sum([]byte(password + user))
	h1hex := fmt.Sprintf("%x", h1)
	h2 := md5.Sum(append([]byte(h1hex), salt...))
	return "md5" + fmt.Sprintf("%x", h2)
}

func (c *conn) handleSASL(payload []byte) error {
	mechanisms := bytesToStrings(payload)
	const mech = "SCRAM-SHA-256"
	found := false
	for _, candidate := range mechanisms {
		if candidate == mech {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("unsupported sasl mechanisms %v", mechanisms)
	}
	nonce, err := randomNonce()
	if err != nil {
		return err
	}
	c.scram = &scramState{username: c.cfg.user, password: c.cfg.password, nonce: nonce}
	// The SASLInitialResponse carries the full client-first-message: the GS2
	// header ("n,," for no channel binding) followed by the client-first-bare.
	initial := append([]byte("n,,"), c.scram.clientFirstBare()...)
	msg := append([]byte(mech), 0)
	msg = binary.BigEndian.AppendUint32(msg, uint32(len(initial)))
	msg = append(msg, initial...)
	return c.writeMessage('p', msg)
}

type scramState struct {
	username    string
	password    string
	nonce       string
	clientFirst string
	serverFirst string
	clientFinal string
	authMessage string
	serverSalt  []byte
	serverIter  int
	serverNonce string
}

func (s *scramState) clientFirstBare() []byte {
	s.clientFirst = fmt.Sprintf("n=%s,r=%s", saslEscape(s.username), s.nonce)
	return []byte(s.clientFirst)
}

func (s *scramState) clientFinalWithoutProof() string {
	return fmt.Sprintf("c=biws,r=%s", s.serverNonce)
}

func (c *conn) handleSASLContinue(payload []byte) error {
	if c.scram == nil {
		return errors.New("scram state missing")
	}
	serverFirst := string(payload)
	c.scram.serverFirst = serverFirst
	parts := strings.Split(serverFirst, ",")
	for _, part := range parts {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch k {
		case "r":
			c.scram.serverNonce = v
		case "s":
			salt, err := base64.StdEncoding.DecodeString(v)
			if err != nil {
				return err
			}
			c.scram.serverSalt = salt
		case "i":
			iter, err := strconv.Atoi(v)
			if err != nil {
				return err
			}
			c.scram.serverIter = iter
		}
	}
	if !strings.HasPrefix(c.scram.serverNonce, c.scram.nonce) {
		return errors.New("scram nonce mismatch")
	}
	clientFinalWithoutProof := c.scram.clientFinalWithoutProof()
	authMessage := c.scram.clientFirstBareString() + "," + serverFirst + "," + clientFinalWithoutProof
	c.scram.authMessage = authMessage
	proof, err := scramClientProof(c.scram.password, c.scram.serverSalt, c.scram.serverIter, authMessage)
	if err != nil {
		return err
	}
	c.scram.clientFinal = clientFinalWithoutProof + ",p=" + base64.StdEncoding.EncodeToString(proof)
	return c.writeMessage('p', []byte(c.scram.clientFinal))
}

func (s *scramState) clientFirstBareString() string { return s.clientFirst }

func (c *conn) handleSASLFinal(payload []byte) error {
	if c.scram == nil {
		return errors.New("scram state missing")
	}
	msg := string(payload)
	for _, part := range strings.Split(msg, ",") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		if k == "v" {
			expected, err := scramServerSignature(c.scram.password, c.scram.serverSalt, c.scram.serverIter, c.scram.authMessage)
			if err != nil {
				return err
			}
			got, err := base64.StdEncoding.DecodeString(v)
			if err != nil {
				return err
			}
			if !hmac.Equal(expected, got) {
				return errors.New("scram server signature mismatch")
			}
		}
	}
	c.scram = nil
	return nil
}

func bytesToStrings(payload []byte) []string {
	parts := strings.Split(string(payload), "\x00")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func randomNonce() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

func saslEscape(s string) string {
	s = strings.ReplaceAll(s, "=", "=3D")
	s = strings.ReplaceAll(s, ",", "=2C")
	return s
}

func scramClientProof(password string, salt []byte, iter int, authMessage string) ([]byte, error) {
	salted := pbkdf2Sha256([]byte(password), salt, iter, sha256.Size)
	clientKey := hmacSHA256(salted, []byte("Client Key"))
	storedKey := sha256.Sum256(clientKey)
	clientSig := hmacSHA256(storedKey[:], []byte(authMessage))
	proof := make([]byte, len(clientKey))
	for i := range clientKey {
		proof[i] = clientKey[i] ^ clientSig[i]
	}
	return proof, nil
}

func scramServerSignature(password string, salt []byte, iter int, authMessage string) ([]byte, error) {
	salted := pbkdf2Sha256([]byte(password), salt, iter, sha256.Size)
	serverKey := hmacSHA256(salted, []byte("Server Key"))
	return hmacSHA256(serverKey, []byte(authMessage)), nil
}

func pbkdf2Sha256(password, salt []byte, iter, keyLen int) []byte {
	hLen := sha256.Size
	n := int(math.Ceil(float64(keyLen) / float64(hLen)))
	out := make([]byte, 0, n*hLen)
	var block [4]byte
	for i := 1; i <= n; i++ {
		binary.BigEndian.PutUint32(block[:], uint32(i))
		u := hmacSHA256(password, append(append([]byte{}, salt...), block[:]...))
		t := make([]byte, len(u))
		copy(t, u)
		for j := 1; j < iter; j++ {
			u = hmacSHA256(password, u)
			for k := range t {
				t[k] ^= u[k]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(data)
	return h.Sum(nil)
}

func parseError(payload []byte) error {
	var message string
	fields := strings.Split(string(payload), "\x00")
	for _, field := range fields {
		if len(field) < 2 {
			continue
		}
		switch field[0] {
		case 'M':
			message = field[1:]
		}
	}
	if message == "" {
		message = "postgres error"
	}
	return errors.New(message)
}

func (c *conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	if c.c != nil {
		return c.c.Close()
	}
	return nil
}

func (c *conn) Begin() (driver.Tx, error) { return c.BeginTx(context.Background(), driver.TxOptions{}) }

func (c *conn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.execLocked("BEGIN"); err != nil {
		return nil, err
	}
	c.txActive = true
	return &tx{c: c}, nil
}

type tx struct{ c *conn }

func (t *tx) Commit() error {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	if _, err := t.c.execLocked("COMMIT"); err != nil {
		return err
	}
	t.c.txActive = false
	return nil
}

func (t *tx) Rollback() error {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	if _, err := t.c.execLocked("ROLLBACK"); err != nil {
		return err
	}
	t.c.txActive = false
	return nil
}

func (c *conn) Prepare(query string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}
func (c *conn) Ping(ctx context.Context) error {
	_, err := c.execContext(ctx, "SELECT 1", nil)
	return err
}

func (c *conn) execContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	return c.execLocked(rewriteQuery(query, args))
}

func (c *conn) execLocked(query string) (driver.Result, error) {
	if err := c.writeMessage('Q', append([]byte(query), 0)); err != nil {
		return nil, err
	}
	var tag string
	for {
		typ, payload, err := c.readMessage()
		if err != nil {
			return nil, err
		}
		switch typ {
		case 'C':
			tag = string(bytesTrimNUL(payload))
		case 'Z':
			return execResult(tag), nil
		case 'E':
			return nil, parseError(payload)
		case 'N', 'S', 'K':
			continue
		default:
			continue
		}
	}
}

func (c *conn) Query(query string, args []driver.Value) (driver.Rows, error) {
	return c.QueryContext(context.Background(), query, toNamed(args))
}

func (c *conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}
	if err := c.writeMessage('Q', append([]byte(rewriteQuery(query, args)), 0)); err != nil {
		return nil, err
	}
	var cols []string
	var rows [][]driver.Value
	for {
		typ, payload, err := c.readMessage()
		if err != nil {
			return nil, err
		}
		switch typ {
		case 'T':
			cols = parseColumns(payload)
		case 'D':
			row, err := parseRow(payload)
			if err != nil {
				return nil, err
			}
			rows = append(rows, row)
		case 'C':
			continue
		case 'Z':
			return &rowsResult{cols: cols, rows: rows}, nil
		case 'E':
			return nil, parseError(payload)
		case 'N', 'S', 'K':
			continue
		default:
			continue
		}
	}
}

func (c *conn) CheckNamedValue(nv *driver.NamedValue) error {
	// Honor driver.Valuer (and normalize standard kinds) before the value
	// reaches query rewriting. Returning nil unconditionally would bypass the
	// converter, so a Valuer such as an array wrapper would be formatted with
	// fmt.Sprint (e.g. "[a]") instead of its intended value ("{a}"). Exotic
	// types the converter cannot handle are left untouched for quoteArg.
	if v, err := driver.DefaultParameterConverter.ConvertValue(nv.Value); err == nil {
		nv.Value = v
	}
	return nil
}

func (c *conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return c.execContext(ctx, query, args)
}

func toNamed(vals []driver.Value) []driver.NamedValue {
	out := make([]driver.NamedValue, len(vals))
	for i, v := range vals {
		out[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return out
}

type execResult string

func (r execResult) LastInsertId() (int64, error) { return 0, errors.New("not supported") }
func (r execResult) RowsAffected() (int64, error) {
	tag := string(r)
	parts := strings.Split(tag, " ")
	if len(parts) == 0 {
		return 0, nil
	}
	last := parts[len(parts)-1]
	if n, err := strconv.ParseInt(last, 10, 64); err == nil {
		return n, nil
	}
	return 0, nil
}

type rowsResult struct {
	cols []string
	rows [][]driver.Value
	idx  int
}

func (r *rowsResult) Columns() []string { return r.cols }
func (r *rowsResult) Close() error      { return nil }
func (r *rowsResult) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.idx]
	r.idx++
	copy(dest, row)
	return nil
}

func parseColumns(payload []byte) []string {
	count := int(binary.BigEndian.Uint16(payload[:2]))
	payload = payload[2:]
	cols := make([]string, 0, count)
	for i := 0; i < count; i++ {
		name, rest := readCString(payload)
		cols = append(cols, name)
		payload = rest[18:]
	}
	return cols
}

func parseRow(payload []byte) ([]driver.Value, error) {
	count := int(binary.BigEndian.Uint16(payload[:2]))
	payload = payload[2:]
	row := make([]driver.Value, 0, count)
	for i := 0; i < count; i++ {
		if len(payload) < 4 {
			return nil, errors.New("invalid row")
		}
		l := int(int32(binary.BigEndian.Uint32(payload[:4])))
		payload = payload[4:]
		if l < 0 {
			row = append(row, nil)
			continue
		}
		if len(payload) < l {
			return nil, errors.New("invalid row data")
		}
		row = append(row, string(payload[:l]))
		payload = payload[l:]
	}
	return row, nil
}

func readCString(b []byte) (string, []byte) {
	for i, c := range b {
		if c == 0 {
			return string(b[:i]), b[i+1:]
		}
	}
	return string(b), nil
}

func bytesTrimNUL(b []byte) []byte {
	if len(b) > 0 && b[len(b)-1] == 0 {
		return b[:len(b)-1]
	}
	return b
}

func rewriteQuery(query string, args []driver.NamedValue) string {
	if len(args) == 0 {
		return query
	}
	var out strings.Builder
	for i := 0; i < len(query); i++ {
		if query[i] != '$' {
			out.WriteByte(query[i])
			continue
		}
		j := i + 1
		for j < len(query) && query[j] >= '0' && query[j] <= '9' {
			j++
		}
		if j == i+1 {
			out.WriteByte(query[i])
			continue
		}
		n, err := strconv.Atoi(query[i+1 : j])
		if err != nil || n < 1 || n > len(args) {
			out.WriteString(query[i:j])
			continue
		}
		out.WriteString(quoteArg(args[n-1].Value))
		i = j - 1
	}
	return out.String()
}

func quoteArg(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case string:
		return quoteString(x)
	case []byte:
		return quoteString(string(x))
	case bool:
		if x {
			return "TRUE"
		}
		return "FALSE"
	case int:
		return strconv.Itoa(x)
	case int8:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case time.Time:
		return quoteString(x.UTC().Format(time.RFC3339Nano))
	case fmt.Stringer:
		return quoteString(x.String())
	default:
		return quoteString(fmt.Sprint(v))
	}
}

func quoteString(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// Ensure unused imports are avoided when the optional SASL path is not exercised.
var _ = math.MaxInt

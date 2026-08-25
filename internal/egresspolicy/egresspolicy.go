package egresspolicy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

type Mode string

type Status string

const (
	ModeAuto   Mode = "auto"
	ModeManual Mode = "manual"
	ModeOff    Mode = "off"

	StatusObserved         Status = "observed"
	StatusDeclared         Status = "declared"
	StatusDisabled         Status = "disabled"
	StatusResolutionFailed Status = "resolution_failed"
)

type Config struct {
	Mode     Mode
	Endpoint string
	Address  string
}

type State struct {
	Mode             Mode       `json:"mode"`
	Status           Status     `json:"status"`
	Address          string     `json:"address,omitempty"`
	ObservedAt       *time.Time `json:"observedAt,omitempty"`
	Interface        string     `json:"interface,omitempty"`
	InterfaceAddress string     `json:"interfaceAddress,omitempty"`
	ResolverEndpoint string     `json:"resolverEndpoint,omitempty"`
	Notes            []string   `json:"notes,omitempty"`
}

func ParseConfigFromEnv(getenv func(string) string) (Config, error) {
	mode := Mode(strings.ToLower(strings.TrimSpace(getenv("WAYPOINT_EGRESS_MODE"))))
	if mode == "" {
		mode = ModeOff
	}
	cfg := Config{Mode: mode, Endpoint: strings.TrimSpace(getenv("WAYPOINT_EGRESS_ENDPOINT")), Address: strings.TrimSpace(getenv("WAYPOINT_EGRESS_ADDRESS"))}
	switch cfg.Mode {
	case ModeAuto:
		if _, err := parseResolverURL(cfg.Endpoint); err != nil {
			return Config{}, err
		}
	case ModeManual:
		if _, err := parseConfiguredAddress(cfg.Address); err != nil {
			return Config{}, err
		}
	case ModeOff:
		cfg.Address = ""
		cfg.Endpoint = ""
	default:
		return Config{}, fmt.Errorf("unsupported WAYPOINT_EGRESS_MODE %q", cfg.Mode)
	}
	return cfg, nil
}

func Resolve(ctx context.Context, cfg Config, client *http.Client) (State, error) {
	now := time.Now().UTC()
	switch cfg.Mode {
	case ModeOff:
		return State{Mode: cfg.Mode, Status: StatusDisabled, Notes: []string{"startup egress discovery disabled; no discovery traffic sent"}}, nil
	case ModeManual:
		addr, err := parseConfiguredAddress(cfg.Address)
		if err != nil {
			return State{}, err
		}
		return State{Mode: cfg.Mode, Status: StatusDeclared, Address: addr.String(), ObservedAt: &now, Notes: []string{"operator-declared egress address accepted at startup"}}, nil
	case ModeAuto:
		endpoint, err := parseResolverURL(cfg.Endpoint)
		if err != nil {
			return State{}, err
		}
		state := State{Mode: cfg.Mode, Status: StatusResolutionFailed, ObservedAt: &now, ResolverEndpoint: endpoint.String(), Notes: []string{"egress resolution failed"}}
		observed, ifaceName, ifaceAddr, err := resolveAuto(ctx, endpoint.String(), client)
		if err != nil {
			state.Notes = append([]string{err.Error()}, state.Notes...)
			return state, nil
		}
		state.Status = StatusObserved
		state.Address = observed
		state.Interface = ifaceName
		state.InterfaceAddress = ifaceAddr
		state.Notes = []string{"resolved through configured endpoint"}
		return state, nil
	default:
		return State{}, fmt.Errorf("unsupported egress mode %q", cfg.Mode)
	}
}

func ResolveFromEnv(ctx context.Context, getenv func(string) string, client *http.Client) (State, error) {
	cfg, err := ParseConfigFromEnv(getenv)
	if err != nil {
		return State{}, err
	}
	return Resolve(ctx, cfg, client)
}

func resolveAuto(ctx context.Context, endpoint string, client *http.Client) (address, ifaceName, ifaceAddr string, err error) {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	var localAddr net.Addr
	transport := &http.Transport{Proxy: nil}
	transport.DialContext = func(dialCtx context.Context, network, addr string) (net.Conn, error) {
		dialer := &net.Dialer{Timeout: 5 * time.Second}
		conn, err := dialer.DialContext(dialCtx, network, addr)
		if err != nil {
			return nil, err
		}
		localAddr = conn.LocalAddr()
		return conn, nil
	}
	client.Transport = transport

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", "", err
	}
	resp, err := client.Do(request)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", "", "", fmt.Errorf("resolver returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", "", "", err
	}
	address, err = parseResolvedAddress(body)
	if err != nil {
		return "", "", "", err
	}
	if localAddr != nil {
		ifaceName, ifaceAddr = interfaceForAddress(localAddr.String())
	}
	return address, ifaceName, ifaceAddr, nil
}

func parseResolverURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("WAYPOINT_EGRESS_ENDPOINT is required for auto mode")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid WAYPOINT_EGRESS_ENDPOINT: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("invalid WAYPOINT_EGRESS_ENDPOINT: unsupported scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, errors.New("invalid WAYPOINT_EGRESS_ENDPOINT: host is required")
	}
	return parsed, nil
}

func parseConfiguredAddress(raw string) (netip.Addr, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Addr{}, errors.New("WAYPOINT_EGRESS_ADDRESS is required for manual mode")
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid WAYPOINT_EGRESS_ADDRESS: %w", err)
	}
	return addr.Unmap(), nil
}

func parseResolvedAddress(body []byte) (string, error) {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "", errors.New("resolver returned an empty body")
	}
	if addr, err := netip.ParseAddr(text); err == nil {
		return addr.Unmap().String(), nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err == nil {
		for _, key := range []string{"address", "ip", "egressAddress", "egressPublicIp", "publicIp"} {
			if raw, ok := payload[key].(string); ok {
				if addr, err := netip.ParseAddr(strings.TrimSpace(raw)); err == nil {
					return addr.Unmap().String(), nil
				}
			}
		}
	}
	return "", errors.New("resolver body did not contain a valid IP address")
}

func interfaceForAddress(addr string) (string, string) {
	parsed := strings.TrimSpace(addr)
	if parsed == "" {
		return "", ""
	}
	if host, _, err := net.SplitHostPort(parsed); err == nil {
		parsed = host
	}
	ip := net.ParseIP(parsed)
	if ip == nil {
		return "", ""
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", ""
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			rangeAddr, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if rangeAddr.IP.Equal(ip) {
				return iface.Name, rangeAddr.IP.String()
			}
		}
	}
	return "", ""
}

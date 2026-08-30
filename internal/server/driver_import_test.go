package server

// The real-PostgreSQL gate tests in this package call sql.Open("postgres", ...),
// which needs the database/sql driver registered. main.go registers it for the
// application binary via this same blank import; the test binary needs its own.
// Without it these tests fail with `sql: unknown driver "postgres"` instead of
// exercising the server against a real database.
import _ "github.com/lib/pq"

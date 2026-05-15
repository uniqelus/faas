package harness

import "context"

// SQLClient is a placeholder for future DB-backed assertions.
// Local-process integration tests do not use SQL in the first iteration.
type SQLClient struct{}

func NewSQLClient() *SQLClient {
	return &SQLClient{}
}

func (c *SQLClient) Ping(_ context.Context) error {
	return nil
}

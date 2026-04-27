package ftp

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	goftp "github.com/jlaffaye/ftp"
)

// Entry represents a file found on the FTP server.
type Entry struct {
	Name      string
	Size      int64
	ModTime   time.Time
	RemotePath string // full FTP path
}

// Client wraps a pool of FTP connections.
type Client struct {
	host    string
	size    int
	mu      sync.Mutex
	pool    []*goftp.ServerConn
}

func NewClient(host string, poolSize int) *Client {
	return &Client{host: host, size: poolSize}
}

func normalizeFTPAddress(host string) string {
	if host == "" {
		return host
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	return net.JoinHostPort(host, "21")
}

// acquire borrows a connection from the pool or dials a new one.
func (c *Client) acquire(ctx context.Context) (*goftp.ServerConn, error) {
	c.mu.Lock()
	if len(c.pool) > 0 {
		conn := c.pool[len(c.pool)-1]
		c.pool = c.pool[:len(c.pool)-1]
		c.mu.Unlock()
		// Verify connection is still alive
		if err := conn.NoOp(); err == nil {
			return conn, nil
		}
		_ = conn.Quit()
	} else {
		c.mu.Unlock()
	}
	return c.dial(ctx)
}

func (c *Client) dial(ctx context.Context) (*goftp.ServerConn, error) {
	address := normalizeFTPAddress(c.host)
	type result struct {
		conn *goftp.ServerConn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := goftp.Dial(address, goftp.DialWithTimeout(30*time.Second))
		if err != nil {
			ch <- result{err: fmt.Errorf("dial %s: %w", address, err)}
			return
		}
		if err := conn.Login("anonymous", "anonymous@datasus"); err != nil {
			_ = conn.Quit()
			ch <- result{err: fmt.Errorf("login: %w", err)}
			return
		}
		ch <- result{conn: conn}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.conn, r.err
	}
}

// release returns a connection to the pool or closes it if the pool is full.
func (c *Client) release(conn *goftp.ServerConn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pool) < c.size {
		c.pool = append(c.pool, conn)
	} else {
		_ = conn.Quit()
	}
}

// ListDir lists all .dbc files in the given FTP directory.
func (c *Client) ListDir(ctx context.Context, dir string) ([]Entry, error) {
	conn, err := c.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer c.release(conn)

	entries, err := conn.List(dir)
	if err != nil {
		return nil, fmt.Errorf("list %q: %w", dir, err)
	}

	var result []Entry
	for _, e := range entries {
		if e.Type != goftp.EntryTypeFile {
			continue
		}
		result = append(result, Entry{
			Name:       e.Name,
			Size:       int64(e.Size),
			ModTime:    e.Time,
			RemotePath: dir + "/" + e.Name,
		})
	}
	return result, nil
}

// Download streams a remote file into the given writer.
func (c *Client) Download(ctx context.Context, remotePath string, dst io.Writer) error {
	conn, err := c.acquire(ctx)
	if err != nil {
		return err
	}
	defer c.release(conn)

	r, err := conn.Retr(remotePath)
	if err != nil {
		return fmt.Errorf("RETR %q: %w", remotePath, err)
	}
	defer r.Close()

	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(dst, r)
		done <- err
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// Close shuts down all pooled connections.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, conn := range c.pool {
		_ = conn.Quit()
	}
	c.pool = nil
}

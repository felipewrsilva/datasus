package ftpclient

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	goftp "github.com/jlaffaye/ftp"
)

// Entry represents a file found on the FTP server.
type Entry struct {
	Name       string
	Size       int64
	ModTime    time.Time
	RemotePath string
}

// Client wraps a pool of FTP connections.
type Client struct {
	host       string
	user       string
	password   string
	size       int
	verifyNoOp bool
	mu         sync.Mutex
	pool       []*goftp.ServerConn
}

func NewClient(host, user, password string, poolSize int) *Client {
	if user == "" {
		user = "anonymous"
	}
	if password == "" {
		password = "anonymous@datasus"
	}
	return &Client{
		host: host, user: user, password: password, size: poolSize, verifyNoOp: true,
	}
}

// SetVerifyNoOp toggles the keep-alive NoOp roundtrip used to validate pooled
// connections on acquire.
func (c *Client) SetVerifyNoOp(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.verifyNoOp = enabled
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

func (c *Client) acquire(ctx context.Context) (*goftp.ServerConn, error) {
	c.mu.Lock()
	verifyNoOp := c.verifyNoOp
	if len(c.pool) > 0 {
		conn := c.pool[len(c.pool)-1]
		c.pool = c.pool[:len(c.pool)-1]
		c.mu.Unlock()
		if !verifyNoOp {
			return conn, nil
		}
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
		if err := conn.Login(c.user, c.password); err != nil {
			_ = conn.Quit()
			ch <- result{err: fmt.Errorf("login: %w", err)}
			return
		}
		ch <- result{conn: conn}
	}()
	select {
	case <-ctx.Done():
		go func() {
			r := <-ch
			if r.conn != nil {
				_ = r.conn.Quit()
			}
		}()
		return nil, ctx.Err()
	case r := <-ch:
		return r.conn, r.err
	}
}

func (c *Client) release(conn *goftp.ServerConn) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pool) < c.size {
		c.pool = append(c.pool, conn)
	} else {
		_ = conn.Quit()
	}
}

// ListDir lists all entries in dir; filter to .dbc files only.
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

	d := strings.TrimRight(strings.TrimSpace(dir), "/")
	var result []Entry
	for _, e := range entries {
		if e.Type != goftp.EntryTypeFile {
			continue
		}
		if !strings.EqualFold(strings.TrimPrefix(filepathExt(e.Name), "."), "dbc") {
			continue
		}
		name := e.Name
		result = append(result, Entry{
			Name:       name,
			Size:       int64(e.Size),
			ModTime:    e.Time,
			RemotePath: FtpPathJoin(d, name),
		})
	}
	return result, nil
}

func filepathExt(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return ""
	}
	return name[i:]
}

// ListSubdirs returns immediate child directory names under dir (not recursive).
func (c *Client) ListSubdirs(ctx context.Context, dir string) ([]string, error) {
	conn, err := c.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer c.release(conn)

	entries, err := conn.List(dir)
	if err != nil {
		return nil, fmt.Errorf("list %q: %w", dir, err)
	}

	var names []string
	for _, e := range entries {
		if e.Type != goftp.EntryTypeFolder {
			continue
		}
		if e.Name == "." || e.Name == ".." {
			continue
		}
		names = append(names, e.Name)
	}
	return names, nil
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

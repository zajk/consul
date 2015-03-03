package api

import (
	crand "crypto/rand"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/hashicorp/consul/testutil"
)

type configCallback func(c *Config)

func makeClient(t *testing.T) (*Client, *testutil.TestServer) {
	return makeClientWithConfig(t, nil, nil)
}

func makeClientWithConfig(
	t *testing.T,
	cb1 configCallback,
	cb2 testutil.ServerConfigCallback) (*Client, *testutil.TestServer) {

	// Make client config
	conf := DefaultConfig()
	if cb1 != nil {
		cb1(conf)
	}

	// Create server
	server := testutil.NewTestServerConfig(t, cb2)
	conf.Address = server.APIAddr

	// Create client
	client, err := NewClient(conf)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	return client, server
}

func testKey() string {
	buf := make([]byte, 16)
	if _, err := crand.Read(buf); err != nil {
		panic(fmt.Errorf("Failed to read random bytes: %v", err))
	}

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%12x",
		buf[0:4],
		buf[4:6],
		buf[6:8],
		buf[8:10],
		buf[10:16])
}

func TestSetQueryOptions(t *testing.T) {
	c, s := makeClient(t)
	defer s.Stop()

	r := c.newRequest("GET", "/v1/kv/foo")
	q := &QueryOptions{
		Datacenter:        "foo",
		AllowStale:        true,
		RequireConsistent: true,
		WaitIndex:         1000,
		WaitTime:          100 * time.Second,
		Token:             "12345",
	}
	r.setQueryOptions(q)

	if r.params.Get("dc") != "foo" {
		t.Fatalf("bad: %v", r.params)
	}
	if _, ok := r.params["stale"]; !ok {
		t.Fatalf("bad: %v", r.params)
	}
	if _, ok := r.params["consistent"]; !ok {
		t.Fatalf("bad: %v", r.params)
	}
	if r.params.Get("index") != "1000" {
		t.Fatalf("bad: %v", r.params)
	}
	if r.params.Get("wait") != "100000ms" {
		t.Fatalf("bad: %v", r.params)
	}
	if r.params.Get("token") != "12345" {
		t.Fatalf("bad: %v", r.params)
	}
}

func TestSetWriteOptions(t *testing.T) {
	c, s := makeClient(t)
	defer s.Stop()

	r := c.newRequest("GET", "/v1/kv/foo")
	q := &WriteOptions{
		Datacenter: "foo",
		Token:      "23456",
	}
	r.setWriteOptions(q)

	if r.params.Get("dc") != "foo" {
		t.Fatalf("bad: %v", r.params)
	}
	if r.params.Get("token") != "23456" {
		t.Fatalf("bad: %v", r.params)
	}
}

func TestRequestToHTTP(t *testing.T) {
	c, s := makeClient(t)
	defer s.Stop()

	r := c.newRequest("DELETE", "/v1/kv/foo")
	q := &QueryOptions{
		Datacenter: "foo",
	}
	r.setQueryOptions(q)
	req, err := r.toHTTP()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if req.Method != "DELETE" {
		t.Fatalf("bad: %v", req)
	}
	if req.URL.RequestURI() != "/v1/kv/foo?dc=foo" {
		t.Fatalf("bad: %v", req)
	}
}

func TestParseQueryMeta(t *testing.T) {
	resp := &http.Response{
		Header: make(map[string][]string),
	}
	resp.Header.Set("X-Consul-Index", "12345")
	resp.Header.Set("X-Consul-LastContact", "80")
	resp.Header.Set("X-Consul-KnownLeader", "true")

	qm := &QueryMeta{}
	if err := parseQueryMeta(resp, qm); err != nil {
		t.Fatalf("err: %v", err)
	}

	if qm.LastIndex != 12345 {
		t.Fatalf("Bad: %v", qm)
	}
	if qm.LastContact != 80*time.Millisecond {
		t.Fatalf("Bad: %v", qm)
	}
	if !qm.KnownLeader {
		t.Fatalf("Bad: %v", qm)
	}
}

func TestAPI_UnixSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.SkipNow()
	}

	tempDir, err := ioutil.TempDir("", "consul")
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	defer os.RemoveAll(tempDir)
	socket := filepath.Join(tempDir, "test.sock")

	c, s := makeClientWithConfig(t, func(c *Config) {
		c.Address = "unix://" + socket
	}, func(c *testutil.TestServerConfig) {
		c.Addresses = &testutil.TestAddressConfig{
			HTTP: "unix://" + socket,
		}
	})
	defer s.Stop()

	agent := c.Agent()

	info, err := agent.Self()
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if info["Config"]["NodeName"] == "" {
		t.Fatalf("bad: %v", info)
	}
}

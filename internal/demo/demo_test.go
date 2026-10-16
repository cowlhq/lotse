package main

import (
	"context"
	"log/slog"
	"net"
	"strconv"
	"testing"
	"time"
)

// The demo starts, sets the hub up, and has its machines online, its
// checks and its history; with two hours of it, to be quick.
func TestDemo(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a hub with a dozen agents")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listen := ln.Addr().String()
	ln.Close()

	ctx, cancel := context.WithCancel(t.Context())
	d, err := start(ctx, options{
		dir: t.TempDir(), listen: listen, history: 2 * time.Hour, version: "0.1.0",
		log: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer func() {
		cancel()
		<-d.hubDone
	}()

	var hosts []struct {
		Host struct {
			Name   string
			Online bool
		}
	}
	if err := d.client.do(ctx, "GET", "/api/hosts", nil, &hosts); err != nil {
		t.Fatal(err)
	}
	if len(hosts) != len(machines(newStory(time.Now()))) {
		t.Errorf("%d hosts", len(hosts))
	}
	for _, h := range hosts {
		if !h.Host.Online {
			t.Errorf("%s is not online", h.Host.Name)
		}
	}
	if len(d.checks) != 15 {
		t.Errorf("%d checks", len(d.checks))
	}
	var series struct {
		Time []int64
	}
	from := time.Now().Add(-2 * time.Hour).Unix()
	if err := d.client.do(ctx, "GET", "/api/hosts/1/series?from="+strconv.FormatInt(from, 10), nil, &series); err != nil {
		t.Fatal(err)
	}
	if len(series.Time) < 60 {
		t.Errorf("%d readings in the last two hours", len(series.Time))
	}
}

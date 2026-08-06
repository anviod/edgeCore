package network

import (
	"testing"

	"github.com/anviod/edgeCore/internal/model"
)

func TestRoutesToRemove(t *testing.T) {
	previous := []model.StaticRoute{
		{Destination: "192.168.2.0", Prefix: 24, Gateway: "192.168.1.1", Enabled: true},
		{Destination: "10.0.0.0", Prefix: 8, Gateway: "192.168.1.254", Enabled: true},
	}
	current := []model.StaticRoute{
		{Destination: "192.168.2.0", Prefix: 24, Gateway: "192.168.1.1", Enabled: true},
	}

	removed := RoutesToRemove(previous, current)
	if len(removed) != 1 {
		t.Fatalf("expected 1 removed route, got %d", len(removed))
	}
	if removed[0].Destination != "10.0.0.0" {
		t.Fatalf("unexpected removed route: %+v", removed[0])
	}
}

func TestParseDarwinRoutes(t *testing.T) {
	input := []byte(`Routing tables

Destination        Gateway            Flags               Netif Expire
default            192.168.1.1        UGScg                 en0
127                127.0.0.1          UCS                   lo0
192.168.2          192.168.1.254      UGSc                  en0
192.168.1          link#8             UCS                   en0
`)

	routes, err := parseDarwinRoutes(input)
	if err != nil {
		t.Fatalf("parseDarwinRoutes() error = %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d: %+v", len(routes), routes)
	}
	if routes[0].Destination != "0.0.0.0" || routes[0].Prefix != 0 || routes[0].Gateway != "192.168.1.1" {
		t.Fatalf("unexpected default route: %+v", routes[0])
	}
	if routes[1].Destination != "192.168.2.0" || routes[1].Prefix != 24 || routes[1].Gateway != "192.168.1.254" {
		t.Fatalf("unexpected static route: %+v", routes[1])
	}
}

func TestNormalizeStaticRoute(t *testing.T) {
	route := NormalizeStaticRoute(model.StaticRoute{Destination: "default", Gateway: "192.168.1.1"})
	if route.Destination != "0.0.0.0" || route.Prefix != 0 {
		t.Fatalf("unexpected normalized route: %+v", route)
	}
}

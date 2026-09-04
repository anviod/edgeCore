package network

import (
	"fmt"
	"strings"

	"github.com/anviod/edgeCore/internal/model"
)

func RouteKey(route model.StaticRoute) string {
	return routeKey(route)
}

func routeKey(route model.StaticRoute) string {
	dest := strings.TrimSpace(route.Destination)
	if dest == "" {
		return ""
	}
	return fmt.Sprintf("%s/%d|%s|%s", dest, route.Prefix, route.Gateway, route.Interface)
}

func NormalizeStaticRoute(route model.StaticRoute) model.StaticRoute {
	return normalizeStaticRoute(route)
}

func RoutesToRemove(previous, current []model.StaticRoute) []model.StaticRoute {
	return routesToRemove(previous, current)
}

func routeKeys(routes []model.StaticRoute) map[string]struct{} {
	keys := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		if key := routeKey(route); key != "" {
			keys[key] = struct{}{}
		}
	}
	return keys
}

func routesToRemove(previous, current []model.StaticRoute) []model.StaticRoute {
	currentKeys := routeKeys(current)
	var removed []model.StaticRoute
	for _, route := range previous {
		if !route.Enabled {
			continue
		}
		if key := routeKey(route); key != "" {
			if _, ok := currentKeys[key]; !ok {
				removed = append(removed, route)
			}
		}
	}
	return removed
}

func normalizeStaticRoute(route model.StaticRoute) model.StaticRoute {
	route.Destination = strings.TrimSpace(route.Destination)
	route.Gateway = strings.TrimSpace(route.Gateway)
	route.Interface = strings.TrimSpace(route.Interface)
	if route.Destination == "default" {
		route.Destination = "0.0.0.0"
		route.Prefix = 0
	}
	return route
}

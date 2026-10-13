package hub

import (
	"testing"

	"lotse.sh/internal/protocol"
)

func TestContainerSlug(t *testing.T) {
	compose := func(project, service, number string) map[string]string {
		return map[string]string{
			"com.docker.compose.project": project, "com.docker.compose.service": service,
			"com.docker.compose.container-number": number,
		}
	}
	for _, c := range []struct {
		c    protocol.Container
		want string
	}{
		{protocol.Container{Name: "shop-app-1", Labels: compose("shop", "app", "1")}, "shop-app"},
		{protocol.Container{Name: "shop-app-2", Labels: compose("shop", "app", "2")}, "shop-app-2"},
		{protocol.Container{Name: "portainer"}, "portainer"},
		{protocol.Container{Name: "x", Labels: map[string]string{"lotse.name": "web"}}, "web"},
	} {
		if got := containerSlug(c.c); got != c.want {
			t.Errorf("%s: %q, want %q", c.c.Name, got, c.want)
		}
	}
}

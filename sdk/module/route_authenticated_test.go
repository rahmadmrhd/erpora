package module

import "testing"

func TestAuthenticatedOption(t *testing.T) {
	if ResolveRouteOptions().Authenticated {
		t.Error("bawaan tidak Authenticated")
	}
	if !ResolveRouteOptions(MaxBodyBytes(10), Authenticated()).Authenticated {
		t.Error("Authenticated() harus menyalakan RouteConfig.Authenticated")
	}
}

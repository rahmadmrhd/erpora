package module

import "testing"

func TestResolveRouteOptions(t *testing.T) {
	if got := ResolveRouteOptions(); got != (RouteConfig{}) {
		t.Errorf("tanpa opsi = %+v", got)
	}
	got := ResolveRouteOptions(MaxBodyBytes(4096), Classify(ClassInternal), nil, Classify(ClassConfidential), Classify(ClassPublic))
	want := RouteConfig{MaxBodyBytes: 4096, MaxBodyBytesSet: true, Classification: ClassConfidential}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestResolveRouteOptions_InvalidClassificationSticks(t *testing.T) {
	got := ResolveRouteOptions(Classify("secret"), Classify(ClassConfidential))
	if got.Classification != "secret" {
		t.Errorf("kelas tidak valid harus tetap tersimpan, got %q", got.Classification)
	}
}

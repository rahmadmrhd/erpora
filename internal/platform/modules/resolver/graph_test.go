package resolver

import (
	"errors"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
)

var vers = Versions{Platform: "0.1.0", SDK: "0.1.0"}

func node(key, version string, deps ...Dep) Node {
	return Node{Key: key, Version: version, PlatformConstraint: "^0.1", SDKConstraint: "^0.1", Deps: deps}
}

func issuesOf(t *testing.T, err error) []Issue {
	t.Helper()
	var re *ResolutionError
	if !errors.As(err, &re) {
		t.Fatalf("error = %v, diharapkan *ResolutionError", err)
	}
	return re.Issues
}

func find(issues []Issue, code string) (Issue, bool) {
	for _, i := range issues {
		if i.Code == code {
			return i, true
		}
	}
	return Issue{}, false
}

func TestResolve_MissingDependency(t *testing.T) {
	_, err := Resolve([]Node{node("acme.a", "1.0.0", Dep{Key: "acme.zz", Constraint: "^1"})}, vers)
	i, ok := find(issuesOf(t, err), "missing_dependency")
	if !ok || i.Module != "acme.a" || i.Dependency != "acme.zz" {
		t.Fatalf("issue = %+v", i)
	}
}

func TestResolve_VersionMismatch(t *testing.T) {
	_, err := Resolve([]Node{
		node("acme.a", "1.0.0", Dep{Key: "acme.b", Constraint: "^2.0"}),
		node("acme.b", "1.4.0"),
	}, vers)
	i, ok := find(issuesOf(t, err), "version_mismatch")
	if !ok || i.Constraint != "^2.0" || i.Found != "1.4.0" || i.Module != "acme.a" || i.Dependency != "acme.b" {
		t.Fatalf("issue = %+v", i)
	}
}

func TestResolve_InvalidConstraint(t *testing.T) {
	_, err := Resolve([]Node{
		node("acme.a", "1.0.0", Dep{Key: "acme.b", Constraint: "^^1"}),
		node("acme.b", "1.0.0"),
	}, vers)
	if _, ok := find(issuesOf(t, err), "invalid_constraint"); !ok {
		t.Fatal("invalid_constraint diharapkan")
	}
}

func TestResolve_Cycles(t *testing.T) {
	cases := []struct {
		name  string
		nodes []Node
		want  []string
	}{
		{"self", []Node{node("a", "1.0.0", Dep{Key: "a", Constraint: "*"})}, []string{"a", "a"}},
		{"two", []Node{
			node("a", "1.0.0", Dep{Key: "b", Constraint: "*"}),
			node("b", "1.0.0", Dep{Key: "a", Constraint: "*"}),
		}, []string{"a", "b", "a"}},
		{"three", []Node{
			node("a", "1.0.0", Dep{Key: "b", Constraint: "*"}),
			node("b", "1.0.0", Dep{Key: "c", Constraint: "*"}),
			node("c", "1.0.0", Dep{Key: "a", Constraint: "*"}),
		}, []string{"a", "b", "c", "a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Resolve(tc.nodes, vers)
			i, ok := find(issuesOf(t, err), "cycle")
			if !ok {
				t.Fatal("cycle diharapkan")
			}
			if !reflect.DeepEqual(i.Path, tc.want) {
				t.Fatalf("Path = %v, want %v", i.Path, tc.want)
			}
		})
	}
}

func TestResolve_Optional(t *testing.T) {
	if _, err := Resolve([]Node{node("acme.a", "1.0.0", Dep{Key: "acme.x", Constraint: "^1", Optional: true})}, vers); err != nil {
		t.Fatalf("opsional hilang harus lolos: %v", err)
	}
	_, err := Resolve([]Node{
		node("acme.a", "1.0.0", Dep{Key: "acme.b", Constraint: "^2", Optional: true}),
		node("acme.b", "1.0.0"),
	}, vers)
	if _, ok := find(issuesOf(t, err), "optional_version_mismatch"); !ok {
		t.Fatal("optional_version_mismatch diharapkan")
	}
}

func TestResolve_Incompatible(t *testing.T) {
	n := node("acme.a", "1.0.0")
	n.PlatformConstraint = "^1.0"
	m := node("acme.b", "1.0.0")
	m.SDKConstraint = "^1.0"
	g, err := Resolve([]Node{n, m}, vers)
	if err != nil {
		t.Fatalf("tidak boleh error: %v", err)
	}
	inc := g.Incompatible()
	if inc["acme.a"][0].Code != "platform_incompatible" {
		t.Errorf("acme.a = %+v", inc["acme.a"])
	}
	if inc["acme.b"][0].Code != "sdk_incompatible" {
		t.Errorf("acme.b = %+v", inc["acme.b"])
	}
}

func TestResolve_ReportsAllIssuesTogether(t *testing.T) {
	_, err := Resolve([]Node{
		node("a", "1.0.0", Dep{Key: "b", Constraint: "*"}),
		node("b", "1.0.0", Dep{Key: "a", Constraint: "*"}),
		node("c", "1.0.0", Dep{Key: "zz", Constraint: "*"}),
	}, vers)
	is := issuesOf(t, err)
	if _, ok := find(is, "cycle"); !ok {
		t.Error("cycle hilang")
	}
	if _, ok := find(is, "missing_dependency"); !ok {
		t.Error("missing_dependency hilang")
	}
	if err.Error() == "" {
		t.Error("Error() kosong")
	}
}

func catalog() []Node {
	core := Dep{Key: "platform.core", Constraint: "^0.1"}
	return []Node{
		node("platform.core", "0.1.0"),
		node("acme.zeta", "1.0.0", core),
		node("acme.alpha", "1.0.0", core),
		node("acme.example", "1.0.0", core),
		node("acme.example-addon", "1.0.0", core, Dep{Key: "acme.example", Constraint: "^1"}),
		node("acme.opt", "1.0.0", core, Dep{Key: "acme.example", Constraint: "^1", Optional: true}),
	}
}

func TestOrder_StableUnderShuffle(t *testing.T) {
	g, err := Resolve(catalog(), vers)
	if err != nil {
		t.Fatal(err)
	}
	want := g.Order()
	if want[0] != "platform.core" || len(want) != 6 {
		t.Fatalf("Order = %v", want)
	}
	if idx := func(k string) int { return slices.Index(want, k) }; idx("acme.example") > idx("acme.opt") {
		t.Errorf("edge opsional harus ikut menentukan urutan: %v", want)
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for range 100 {
		ns := catalog()
		rng.Shuffle(len(ns), func(i, j int) { ns[i], ns[j] = ns[j], ns[i] })
		g2, err := Resolve(ns, vers)
		if err != nil {
			t.Fatal(err)
		}
		if got := g2.Order(); !reflect.DeepEqual(got, want) {
			t.Fatalf("Order = %v, want %v", got, want)
		}
	}
}

func TestRequiredDependents(t *testing.T) {
	g, err := Resolve(catalog(), vers)
	if err != nil {
		t.Fatal(err)
	}
	if got := g.RequiredDependents("acme.example"); !reflect.DeepEqual(got, []string{"acme.example-addon"}) {
		t.Errorf("RequiredDependents = %v", got)
	}
	if got := g.RequiredDependencies("acme.example-addon"); !reflect.DeepEqual(got, []string{"platform.core", "acme.example"}) {
		t.Errorf("RequiredDependencies = %v", got)
	}
	if got := g.RequiredDependents("platform.core"); len(got) != 5 {
		t.Errorf("dependen core = %v", got)
	}
	if _, ok := g.Node("acme.alpha"); !ok {
		t.Error("Node tidak ditemukan")
	}
}

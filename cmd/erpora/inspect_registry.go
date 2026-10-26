package main

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"erpora/internal/app"
	"erpora/internal/platform/modules/registry"
)

// Perintah inspect berikut tidak membutuhkan database maupun environment:
// hanya katalog modul terkompilasi (app.BuildRegistry).

type routeView struct {
	Module     string `json:"module"`
	Method     string `json:"method"`
	Pattern    string `json:"pattern"`
	Permission string `json:"permission"`
	Public     bool   `json:"public"`
}

type moduleView struct {
	Key          string   `json:"key"`
	Slug         string   `json:"slug"`
	Version      string   `json:"version"`
	Dependencies []string `json:"dependencies"`
	Compatible   bool     `json:"compatible"`
	Problems     []string `json:"incompatibleCodes,omitempty"`
}

type permissionView struct {
	Module     string `json:"module"`
	Permission string `json:"permission"`
}

func newInspectRoutesCmd(use string, grouped bool) *cobra.Command {
	return inspectCmd(use, "Daftar route modul terdaftar", grouped, func(w io.Writer, reg *registry.Registry, asJSON bool) error {
		views := routeViews(reg)
		if asJSON {
			return writeJSON(w, views)
		}
		return writeTable(w, []string{"MODULE", "METHOD", "PATTERN", "PERMISSION", "PUBLIC"}, func(row func(...string)) {
			for _, v := range views {
				row(v.Module, v.Method, v.Pattern, v.Permission, fmt.Sprint(v.Public))
			}
		})
	})
}

func newInspectModulesCmd() *cobra.Command {
	return inspectCmd("modules", "Daftar modul terkompilasi (urutan topologis)", false, func(w io.Writer, reg *registry.Registry, asJSON bool) error {
		views := moduleViews(reg)
		if asJSON {
			return writeJSON(w, views)
		}
		return writeTable(w, []string{"KEY", "SLUG", "VERSION", "DEPENDENCIES", "COMPATIBLE"}, func(row func(...string)) {
			for _, v := range views {
				compat := "yes"
				if !v.Compatible {
					compat = "no (" + strings.Join(v.Problems, ",") + ")"
				}
				deps := "-"
				if len(v.Dependencies) > 0 {
					deps = strings.Join(v.Dependencies, ", ")
				}
				row(v.Key, v.Slug, v.Version, deps, compat)
			}
		})
	})
}

// View selalu non-nil agar --json mencetak [] (bukan null) untuk daftar kosong.
func routeViews(reg *registry.Registry) []routeView {
	views := []routeView{}
	for _, r := range reg.Routes() {
		views = append(views, routeView{Module: r.Module, Method: r.Method, Pattern: r.Pattern, Permission: r.Permission, Public: r.Public})
	}
	return views
}

func moduleViews(reg *registry.Registry) []moduleView {
	incompatible := reg.Graph().Incompatible()
	views := []moduleView{}
	for _, e := range reg.Entries() {
		v := moduleView{Key: e.Manifest.Key, Slug: e.Manifest.Slug, Version: e.Manifest.Version, Dependencies: []string{}, Compatible: true}
		for _, d := range e.Manifest.Dependencies {
			v.Dependencies = append(v.Dependencies, d.Module+" "+d.Version)
		}
		for _, is := range incompatible[e.Manifest.Key] {
			v.Compatible = false
			v.Problems = append(v.Problems, is.Code)
		}
		views = append(views, v)
	}
	return views
}

func permissionViews(reg *registry.Registry) []permissionView {
	views := []permissionView{}
	for _, e := range reg.Entries() {
		caps := slices.Clone(e.Manifest.Capabilities)
		slices.Sort(caps)
		for _, c := range caps {
			views = append(views, permissionView{Module: e.Manifest.Key, Permission: c})
		}
	}
	return views
}

func newInspectPermissionsCmd(use string, grouped bool) *cobra.Command {
	return inspectCmd(use, "Daftar permission (capability) per modul", grouped, func(w io.Writer, reg *registry.Registry, asJSON bool) error {
		views := permissionViews(reg)
		if asJSON {
			return writeJSON(w, views)
		}
		return writeTable(w, []string{"MODULE", "PERMISSION"}, func(row func(...string)) {
			for _, v := range views {
				row(v.Module, v.Permission)
			}
		})
	})
}

// inspectCmd membuat command read-only atas katalog modul dengan flag --json.
// grouped=true menempatkannya di grup inspect root (untuk alias seperti route:list).
func inspectCmd(use, short string, grouped bool, render func(io.Writer, *registry.Registry, bool) error) *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reg, err := app.BuildRegistry()
			if err != nil {
				return &app.ExitError{Code: 2, Err: fmt.Errorf("katalog modul tidak valid: %w", err)}
			}
			return render(cmd.OutOrStdout(), reg, asJSON)
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "keluaran JSON")
	if grouped {
		c.GroupID = groupInspect
	}
	return c
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeTable(w io.Writer, header []string, fill func(row func(...string))) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	fill(func(cols ...string) { fmt.Fprintln(tw, strings.Join(cols, "\t")) })
	return tw.Flush()
}

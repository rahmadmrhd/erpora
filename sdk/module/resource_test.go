package module

import (
	"reflect"
	"testing"
)

func TestParseManifest_ResourcesDimensionsRoles(t *testing.T) {
	input := []byte(`{
  "schema_version": "1", "key": "acme.inventory", "slug": "inventory",
  "resources": [{
    "key": "inventory.item", "label": "resource.item",
    "actions": ["read", "update"],
    "custom_actions": [{"key": "adjust_price", "label": "action.adjust_price"}],
    "owner_field": "created_by",
    "scope_fields": {"inventory.warehouse": "warehouse_id"},
    "field_groups": [
      {"key": "general", "label": "fieldGroup.general", "classification": "internal", "fields": ["name"], "default": true},
      {"key": "pricing", "label": "fieldGroup.pricing", "classification": "confidential", "sensitive": true}
    ],
    "audit": {"data_changes": true}
  }],
  "scope_dimensions": [{"key": "inventory.warehouse", "label": "dimension.warehouse", "source": "inventory.warehouse"}],
  "default_roles": [{
    "key": "inventory.manager", "label": "role.manager",
    "permissions": [{"resource": "inventory.item", "actions": ["read"], "scope": ["owner", "assigned:inventory.warehouse"]}],
    "field_access": [{"resource": "inventory.item", "field_group": "pricing", "access": "read"}]
  }]
}`)
	m, err := ParseManifest(input)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if len(m.Resources) != 1 {
		t.Fatalf("Resources = %+v", m.Resources)
	}
	r := m.Resources[0]
	if r.Key != "inventory.item" || r.Label != "resource.item" || r.OwnerField != "created_by" ||
		r.ScopeFields["inventory.warehouse"] != "warehouse_id" || r.Audit == nil || !r.Audit.DataChanges {
		t.Errorf("resource = %+v", r)
	}
	if len(r.CustomActions) != 1 || r.CustomActions[0].Key != "adjust_price" || r.CustomActions[0].Label != "action.adjust_price" {
		t.Errorf("CustomActions = %+v", r.CustomActions)
	}
	if len(r.FieldGroups) != 2 || !r.FieldGroups[0].Default || r.FieldGroups[0].Classification != "internal" ||
		!reflect.DeepEqual(r.FieldGroups[0].Fields, []string{"name"}) || !r.FieldGroups[1].Sensitive {
		t.Errorf("FieldGroups = %+v", r.FieldGroups)
	}
	if len(m.ScopeDimensions) != 1 || m.ScopeDimensions[0].Source != "inventory.warehouse" || m.ScopeDimensions[0].Label != "dimension.warehouse" {
		t.Errorf("ScopeDimensions = %+v", m.ScopeDimensions)
	}
	if len(m.DefaultRoles) != 1 {
		t.Fatalf("DefaultRoles = %+v", m.DefaultRoles)
	}
	dr := m.DefaultRoles[0]
	if dr.Key != "inventory.manager" || len(dr.Permissions) != 1 || dr.Permissions[0].Resource != "inventory.item" ||
		!reflect.DeepEqual(dr.Permissions[0].Scope, []string{"owner", "assigned:inventory.warehouse"}) ||
		len(dr.FieldAccess) != 1 || dr.FieldAccess[0].FieldGroup != "pricing" || dr.FieldAccess[0].Access != "read" {
		t.Errorf("DefaultRole = %+v", dr)
	}
}

func TestManifestWithoutResourcesStillParses(t *testing.T) {
	m, err := ParseManifest([]byte(`{"schema_version":"1","key":"acme.old","slug":"old","capabilities":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Resources != nil || m.ScopeDimensions != nil || m.DefaultRoles != nil || len(m.Permissions()) != 0 {
		t.Errorf("manifest lama harus tanpa resource: %+v", m)
	}
}

func TestCapabilitiesDerivedFromResources(t *testing.T) {
	m, err := ParseManifest([]byte(`{"schema_version":"1","key":"acme.inventory","slug":"inventory","resources":[
  {"key":"inventory.item","label":"resource.item","actions":["read","create"],
   "custom_actions":[{"key":"adjust_price","label":"action.adjust_price"}],
   "field_groups":[{"key":"general","label":"fieldGroup.general","default":true}]},
  {"key":"inventory.ping","label":"resource.ping","actions":["read"],
   "field_groups":[{"key":"general","label":"fieldGroup.general","default":true}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"inventory.item.read", "inventory.item.create", "inventory.item.adjust_price", "inventory.ping.read"}
	if got := m.Permissions(); !reflect.DeepEqual(got, want) {
		t.Errorf("Permissions = %v, want %v", got, want)
	}
	if !IsStandardAction("approve") || IsStandardAction("adjust_price") {
		t.Error("IsStandardAction salah")
	}
}

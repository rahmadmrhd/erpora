package check

import (
	"erpora/internal/devtools/metadatatools/load"
	"erpora/internal/devtools/metadatatools/naming"
	"erpora/internal/devtools/metadatatools/resolve"
)

// Kode temuan metadata:check. Kode dari pemuat (load.Code*) dan resolver (resolve.Code*) diteruskan apa adanya.
const (
	// Penamaan (Requirement 1.4, 1.7, 1.8).
	CodeNameNotCamelCase        = "name_" + string(naming.CodeNotCamelCase)
	CodeNameConsecutiveCapitals = "name_" + string(naming.CodeConsecutiveCapitals)
	CodeNameTooLong             = "name_" + string(naming.CodeTooLong)
	CodeNameReservedSQL         = "name_" + string(naming.CodeReservedSQL)
	CodeNameReservedGo          = "name_" + string(naming.CodeReservedGo)
	CodeNameReservedTS          = "name_" + string(naming.CodeReservedTS)
	CodeNameRoundTrip           = "name_" + string(naming.CodeRoundTrip)
	CodeNameCollision           = "name_" + string(naming.CodeCollision)
	CodeNameInvalid             = "name_invalid"

	// Field, tipe, dan batasan (Requirement 2).
	CodeFieldAttrNotForType     = "field_attr_not_for_type"
	CodeFieldAttrRequired       = "field_attr_required"
	CodeFieldScalePrecision     = "field_scale_exceeds_precision"
	CodeFieldRange              = "field_range"
	CodeFieldPatternInvalid     = "field_pattern_invalid"
	CodeFieldAggregatableType   = "field_aggregatable_type"
	CodeFieldDefaultInvalid     = "field_default_invalid"
	CodeFieldHighlyConfidential = "field_highly_confidential"
	CodeFieldSensitiveName      = "field_sensitive_name"
	CodeFieldReadOnlyRequired   = "field_readonly_required"
	CodeFieldDeprecatedTarget   = "field_deprecated_target"

	// unmanagedColumns dan generate.crud (Requirement 2.9; konsep menunggu konfirmasi pemilik produk).
	CodeUnmanagedColumnManaged   = "unmanaged_column_managed"
	CodeUnmanagedColumnDuplicate = "unmanaged_column_duplicate"
	CodeCrudWithUnmanaged        = "crud_with_unmanaged_columns"

	// Kondisi (Requirement 4).
	CodeConditionInvalid      = "condition_invalid"
	CodeConditionUnknownField = "condition_unknown_field"
	CodeConditionUnknownRef   = "condition_unknown_ref"
	CodeConditionOpType       = "condition_op_type"
	CodeConditionValueType    = "condition_value_type"
	CodeConditionRefType      = "condition_ref_type"
	CodeConditionCycle        = "condition_cycle"
	CodeRuleScopeConflict     = "rule_scope_conflict"

	// Peringatan (Requirement 9.8, 2.3).
	CodeRuleRefsNonDefaultGroup = "rule_refs_nondefault_group"
	CodeRelationFKNoIndex       = "relation_fk_no_index"

	// Accessor dan templat label (Requirement 5).
	CodeAccessorNotReadOnly             = "accessor_not_readonly"
	CodeAccessorExprInvalid             = "accessor_expr_invalid"
	CodeAccessorUnknownField            = "accessor_unknown_field"
	CodeAccessorFlagNotStored           = "accessor_flag_not_stored"
	CodeAccessorFuncNeedsGroup          = "accessor_func_needs_group"
	CodeAccessorFuncNeedsClassification = "accessor_func_needs_classification"
	CodeAccessorLooserClassification    = "accessor_looser_classification"
	CodeAccessorLooserGroup             = "accessor_looser_group"
	CodeAccessorCycle                   = "accessor_cycle"
	CodeLabelTemplateInvalid            = "label_template_invalid"
	CodeLabelUnknownField               = "label_unknown_field"
	CodeLabelFieldGroup                 = "label_field_group"
	CodeLabelFieldClassification        = "label_field_classification"

	// Relasi dan indeks (Requirement 6).
	CodeRelationTargetMissing       = "relation_target_missing"
	CodeRelationModuleDependency    = "relation_module_dependency"
	CodeRelationEmbeddedModule      = "relation_embedded_module"
	CodeRelationFilterUnknownField  = "relation_filter_unknown_field"
	CodeRelationFilterNotFilterable = "relation_filter_not_filterable"
	CodeRelationFilterUnknownRef    = "relation_filter_unknown_ref"
	CodeRelationFilterCycle         = "relation_filter_cycle"
	CodeRelationColumnCollision     = "relation_column_collision"
	CodeIndexUnknownField           = "index_unknown_field"

	// Manifest (Requirement 7).
	CodeManifestResourceMissing  = "manifest_resource_missing"
	CodeManifestCapabilityPrefix = "manifest_capability_prefix"
	CodeManifestGroupUnknown     = "manifest_group_unknown"
	CodeManifestFieldsMismatch   = "manifest_fields_mismatch"
	CodeManifestActionMissing    = "manifest_action_missing"
	CodeOwnerFieldUnknown        = "owner_field_unknown"
	CodeOwnerFieldType           = "owner_field_type"
	CodeScopeFieldUnknown        = "scope_field_unknown"
	CodeScopeFieldType           = "scope_field_type"
	CodeOwnerManifestMismatch    = "owner_manifest_mismatch"

	// Deprecated, salinan, dan penguncian (Requirement 12.6, 13.12, 14).
	CodeMigrateFromUnknown        = "migrate_from_unknown"
	CodeMigrateFromGroup          = "migrate_from_group"
	CodeMigrateFromClassification = "migrate_from_classification"
	CodeSnapshotRelationType      = "snapshot_relation_type"
	CodeSnapshotToUnknown         = "snapshot_to_unknown"
	CodeSnapshotToNotReadOnly     = "snapshot_to_not_readonly"
	CodeSnapshotToExplicit        = "snapshot_to_explicit"
	CodeSnapshotFromUnknown       = "snapshot_from_unknown"
	CodeSnapshotSourceFlag        = "snapshot_source_flag"
	CodeSnapshotLooser            = "snapshot_looser_classification"
	CodeSnapshotGroupDowngrade    = "snapshot_group_downgrade"
	CodeLockWhenNotSQL            = "lockwhen_not_sql"

	// Internal.
	CodeBuiltinInvalid = "builtin_invalid"
	CodeManifestParse  = "manifest_parse"
)

// Kode yang diteruskan dari resolver (nama sama agar mudah dirujuk pemanggil).
const CodePrefixInvalid = resolve.CodePrefixInvalid

// Kode yang diteruskan dari pemuat.
const CodeSchema = load.CodeSchema

// Efektivitas scope aturan dan kondisi (Requirement 4.6).
const (
	ScopeBoth   = "both"
	ScopeServer = "server"
)

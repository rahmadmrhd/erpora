package app

import "erpora/internal/platform/modules/lifecycle"

// lifecycleTestHooks memungkinkan test integrasi memasang seam pada lifecycle.Deps (mis. AfterStep untuk
// simulasi crash). Default no-op; file test berbuild-tag integration menimpanya.
var lifecycleTestHooks = func(*lifecycle.Deps) {}

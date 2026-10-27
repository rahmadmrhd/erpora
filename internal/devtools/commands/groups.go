package commands

// Judul grup tampil di `erpora list`; ID grup didaftarkan oleh package devtools (kecuali GroupModule,
// milik root).
var groupTitles = map[string]string{
	GroupMake:      "Make",
	GroupDB:        "Database",
	GroupContracts: "Kontrak",
	GroupModule:    "Modul",
	GroupDevtools:  "Developer tools",
}

// GroupTitle mengembalikan judul grup (kosong bila tidak dikenal).
func GroupTitle(id string) string { return groupTitles[id] }

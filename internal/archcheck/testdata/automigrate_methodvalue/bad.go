package fixture

type X struct{}

type db interface{ AutoMigrate(...any) error }

func migrate(d db) {
	m := d.AutoMigrate // method value: tidak ada CallExpr langsung
	_ = m(&X{})
}

package fixture

type X struct{}

func migrate(db interface{ AutoMigrate(...any) error }) {
	_ = db.AutoMigrate(&X{})
}

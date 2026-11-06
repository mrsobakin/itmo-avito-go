package trip

const (
	tripsTable   = "trips"
	historyTable = "trip_status_history"
)

type Repository struct {
	db db
}

func New(db db) *Repository {
	return &Repository{db: db}
}

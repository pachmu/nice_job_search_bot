package bot

type BotDB interface {
	CreateCareers(url string) error
	GetAllCareers() ([]Career, error)
	CheckExists(url string) (bool, error)
	MarkRead(url string) error
	MarkLiked(url string)
}

type Career struct {
	ID    int
	URL   string
	Seen  bool
	Liked bool
}

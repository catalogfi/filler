package bot

type Bot interface {
	UpdateStrategy(newStra config) error
}

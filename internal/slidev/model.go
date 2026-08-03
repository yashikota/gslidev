package slidev

type Deck struct {
	Title          string
	PresentationID string
	FolderID       string
	Slides         []Slide
}

type Slide struct {
	Notes      string
	Screenshot []byte
	GIFs       []Media
}

type Media struct {
	Data   []byte
	MIME   string
	Source string
	X      float64
	Y      float64
	Width  float64
	Height float64
}

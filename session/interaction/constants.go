package interaction

const (
	backspaceChar    = 8
	deleteChar       = 127
	enterChar        = 13
	escapeChar       = 27
	ctrlC            = 3
	forwardSlash     = '/'
	minPrintableChar = 32
	maxPrintableChar = 126

	minSlugLength = 3
	maxSlugLength = 20

	clearScreen    = "\033[H\033[2J"
	clearLine      = "\033[K"
	clearToLineEnd = "\r\033[K"
	backspaceSeq   = "\b \b"

	minBoxWidth  = 50
	paddingRight = 4
)

var forbiddenSlugs = []string{
	"ping",
}

package file

// File defines file name with its content
type File interface {
	Name() string
	Content() []byte
}

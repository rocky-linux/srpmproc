package blob

type Storage interface {
	Write(path string, content []byte)
	Read(path string) []byte
	Exists(path string) bool
}

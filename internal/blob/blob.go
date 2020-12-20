package blob

type Storage interface {
	Write(path string, content []byte)
}

package frontier

type FetchQueue struct {
	Items chan string
}

func NewFetchQueue(maxSize int) *FetchQueue {
	return &FetchQueue{
		Items: make(chan string, maxSize),
	}
}

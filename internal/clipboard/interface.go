package clipboard

// MonitorInterface defines the contract for clipboard monitoring.
// The concrete Monitor type satisfies this interface, allowing callers
// to mock clipboard operations in tests.
type MonitorInterface interface {
	Start()
	Stop()
	ListHistory() []Entry
	GetImage(index int) (string, error)
	GetLatestImage() (string, error)
}

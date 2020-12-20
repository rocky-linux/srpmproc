package internal

type ImportMode interface {
	RetrieveSource(pd *ProcessData) *modeData
	WriteSource(md *modeData)
	PostProcess(md *modeData)
	ImportName(pd *ProcessData, md *modeData) string
}

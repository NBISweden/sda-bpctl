package models

type FileInfo struct {
	AccessionID string `json:"accessionID,omitempty"`
	FileID      string `json:"fileID"`
	InboxPath   string `json:"inboxPath"`
	Status      string `json:"fileStatus"`
	CreateAt    string `json:"createAt"`
}

type DatasetInfo struct {
	Status        string `json:"status"`
	CreatedAt     string `json:"createdAt"`
	NumberOfFiles int    `json:"numberOfFiles"`
}

// DatasetFileInfo is a file mapped to a dataset, as listed by
// GET /dataset/<datasetID>/files.
type DatasetFileInfo struct {
	FileID      string `json:"id"`
	AccessionID string `json:"accessionID"`
	Path        string `json:"path"`
}

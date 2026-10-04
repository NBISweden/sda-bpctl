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

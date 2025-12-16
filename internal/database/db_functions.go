package database

import (
	"database/sql"
	"fmt"

	"github.com/NBISweden/sda-bpctl/internal/models"
	"github.com/cenkalti/backoff/v4"
)

func (dbs *PostgresDb) GetUserFiles(userID, pathPrefix string, allData bool) ([]models.FileInfo, error) {
	files := []models.FileInfo{}
	db := dbs.db

	const query = `SELECT f.id, f.submission_file_path, f.stable_id, e.event, f.created_at FROM sda.files f
LEFT JOIN (SELECT DISTINCT ON (file_id) file_id, started_at, event FROM sda.file_event_log ORDER BY file_id, started_at DESC) e ON f.id = e.file_id
WHERE f.submission_user = $1 and f.submission_file_path LIKE $2
AND NOT EXISTS (SELECT 1 FROM sda.file_dataset d WHERE f.id = d.file_id);`

	var rows *sql.Rows
	err := backoff.Retry(func() error {
		var err error
		rows, err = db.Query(query, userID, fmt.Sprintf("%s%%", pathPrefix))
		return err
	}, backoff.NewExponentialBackOff())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var accessionID sql.NullString
		fi := models.FileInfo{}
		err := rows.Scan(&fi.FileID, &fi.InboxPath, &accessionID, &fi.Status, &fi.CreateAt)
		if err != nil {
			return nil, err
		}

		if allData {
			fi.AccessionID = accessionID.String
		}

		if fi.Status != "disabled" {
			files = append(files, fi)
		}
	}

	return files, nil
}

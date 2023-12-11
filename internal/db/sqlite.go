package db

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pachmu/nice_job_search_bot/internal/bot"
	"log"
)

type SQLiteDB struct {
	db *sql.DB
}

func NewSQLiteDB(path string) (*SQLiteDB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	createStudentTableSQL := `CREATE TABLE IF NOT EXISTS careers (
		"id" integer NOT NULL PRIMARY KEY AUTOINCREMENT,		
		"url" TEXT,
		"seen" BOOL,
		"liked" BOOL		
	  );` // SQL Statement for Create Table

	statement, err := db.Prepare(createStudentTableSQL) // Prepare SQL Statement
	if err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}
	_, err = statement.Exec() // Execute SQL Statements
	if err != nil {
		return nil, fmt.Errorf("failed to execute statement: %w", err)
	}
	log.Println("table created")
	return &SQLiteDB{
		db: db,
	}, nil
}

func (s *SQLiteDB) CreateCareers(url string) error {
	insertCareersSQL := `INSERT INTO careers(url, seen, liked) VALUES (?, ?, ?)`
	statement, err := s.db.Prepare(insertCareersSQL) // Prepare statement.
	// This is good to avoid SQL injections
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)

	}
	_, err = statement.Exec(url, true, false)
	if err != nil {
		return fmt.Errorf("failed to exaecute statement: %w", err)
	}

	return nil
}

func (s *SQLiteDB) GetAllCareers() ([]bot.Career, error) {
	row, err := s.db.Query("SELECT * FROM careers")
	if err != nil {
		return nil, fmt.Errorf("failed to query careers list: %w", err)
	}
	defer row.Close()
	var careers []bot.Career
	for row.Next() {
		career := bot.Career{}
		err = row.Scan(&career.ID, &career.URL, &career.Seen, &career.Liked)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		careers = append(careers, career)
	}

	return careers, nil
}

func (s *SQLiteDB) CheckExists(url string) (bool, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SQLiteDB) MarkRead(url string) error {
	//TODO implement me
	panic("implement me")
}

func (s *SQLiteDB) MarkLiked(url string) {
	//TODO implement me
	panic("implement me")
}

package main

import (
	"bytes"
	"cert-api/dto"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

var db *sql.DB
var err error

var connStr = "user=postgres password=4568 dbname=postgres host=localhost sslmode=disable"

func main() {
	r := mux.NewRouter()

	// DB connection
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("DB connection failed:", err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatal("DB ping failed:", err)
	}
	fmt.Println("✅ Connected to DB!")

	// Serve home page
	r.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./static/home.html")
	}).Methods("GET")

	// Static files
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))

	// API routes
	api := r.PathPrefix("/worldcert").Subrouter()
	api.HandleFunc("/createcert", createCert).Methods("POST")
	api.HandleFunc("/getallcerts", getAllCerts).Methods("GET")
	api.HandleFunc("/getcertbyid/{id}", getCert).Methods("GET")
	api.HandleFunc("/updatecertbyid/{id}", updateCert).Methods("PUT")
	api.HandleFunc("/deletecertbyid/{id}", deleteCert).Methods("DELETE")

	// Debug route
	r.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	}).Methods("GET")

	fmt.Println("🚀 Server running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

func createCert(w http.ResponseWriter, r *http.Request) {
	fmt.Println("CreateCert method entered")
	var cert dto.Certificate
	if err := json.NewDecoder(r.Body).Decode(&cert); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	convertReq := dto.ConvertRequest{ //preparing req for data for the currency converter
		Amount:      cert.PremiumAmount,
		CountryCode: strings.ToUpper(cert.Country),
	}

	jsonDataofconvertreq, err := json.Marshal(convertReq)
	if err != nil {
		http.Error(w, "Failed to marshal JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // ONLY for localhost
		},
	}
	fmt.Println("Before Calling After calling Currency Converter")

	reqPost, err := http.NewRequest("POST", "http://localhost:8081/api/currency", bytes.NewBuffer(jsonDataofconvertreq))
	if err != nil {
		http.Error(w, "Failed to create POST request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	reqPost.Header.Set("Content-Type", "application/json")

	responsefromcurconv, err := httpClient.Do(reqPost)
	if err != nil {
		http.Error(w, "Failed to call currency API: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer responsefromcurconv.Body.Close()

	var currencyResp dto.ConvertResponse
	if err := json.NewDecoder(responsefromcurconv.Body).Decode(&currencyResp); err != nil {
		http.Error(w, "Failed to decode currency API response", http.StatusInternalServerError)
		return
	}
	cert.PremiumAmount = currencyResp.ConvertedAmount
	query := `
	INSERT INTO worldcerts 
	(labname, medicinename, country, noofparticipants, placebo, participantbelongsto, currencytype, premiumamount, category)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`
	errone := db.QueryRow(query,
		cert.Labname,
		cert.MedicineName,
		cert.Country,
		cert.NoofParticipants,
		cert.Placebo,
		cert.Participantbelongsto,
		cert.Currencytype,
		cert.PremiumAmount,
		cert.Category,
	).Scan(&cert.Id)
	if errone != nil {
		http.Error(w, errone.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cert)
}

func getAllCerts(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`SELECT id, labname, medicinename, country, noofparticipants, placebo, participantbelongsto, currencytype, premiumamount, category FROM worldcerts`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var allCerts []dto.Certificate
	for rows.Next() {
		var cert dto.Certificate
		if err := rows.Scan(
			&cert.Id,
			&cert.Labname,
			&cert.MedicineName,
			&cert.Country,
			&cert.NoofParticipants,
			&cert.Placebo,
			&cert.Participantbelongsto,
			&cert.Currencytype,
			&cert.PremiumAmount,
			&cert.Category,
		); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		allCerts = append(allCerts, cert)

	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(allCerts)
}

func getCert(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var cert dto.Certificate

	query := `SELECT id, labname, medicinename, country, noofparticipants, placebo, participantbelongsto, currencytype, premiumamount, category FROM worldcerts WHERE id = $1`
	err := db.QueryRow(query, id).Scan(
		&cert.Id,
		&cert.Labname,
		&cert.MedicineName,
		&cert.Country,
		&cert.NoofParticipants,
		&cert.Placebo,
		&cert.Participantbelongsto,
		&cert.Currencytype,
		&cert.PremiumAmount,
		&cert.Category,
	)
	if err != nil {
		http.Error(w, "Certificate not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cert)
}

func updateCert(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var cert dto.Certificate

	if err := json.NewDecoder(r.Body).Decode(&cert); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	query := `UPDATE worldcerts SET labname=$1, medicinename=$2, country=$3, noofparticipants=$4, placebo=$5, participantbelongsto=$6, currencytype=$7, premiumamount=$8, category=$9 WHERE id=$10`
	_, err = db.Exec(query,
		cert.Labname,
		cert.MedicineName,
		cert.Country,
		cert.NoofParticipants,
		cert.Placebo,
		cert.Participantbelongsto,
		cert.Currencytype,
		cert.PremiumAmount,
		cert.Category,
		id,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	cert.Id, _ = strconv.Atoi(id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(cert)
}

func deleteCert(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	result, err := db.Exec(`DELETE FROM worldcerts WHERE id = $1`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Println(deleteCert)

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Certificate not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": fmt.Sprintf("Certificate with id %s deleted successfully", id),
	})
}

package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
)

type ConvertRequest struct {
	Amount      float64 `json:"amount"`
	CountryCode string  `json:"country_code"` // e.g., "US", "IN", "GB"
}

type ConvertResponse struct {
	OriginalAmount  float64 `json:"original_amount"`
	ConvertedAmount float64 `json:"converted_amount"`
	CurrencyCode    string  `json:"currency_code"`
	CurrencyName    string  `json:"currency_name"`
}

type Asset struct {
	Name    string
	Content []byte
}

var db *sql.DB

func InitDB() {
	var err error
	connStr := "host=localhost port=5432 user=postgres password=4568 dbname=postgres sslmode=disable"
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	err = db.Ping()
	if err != nil {
		log.Fatal("DB connection failed:", err)
	}
}

func SaveConversion(originalAmount, convertedAmount float64, countryCode, currencyCode string) error {
	log.Println("Inserting to DB...")
	_, err := db.Exec(`
        INSERT INTO conversions (original_amount, converted_amount, country_code, currency_code, created_at)
        VALUES ($1, $2, $3, $4, NOW())`,
		originalAmount, convertedAmount, countryCode, currencyCode,
	)
	if err != nil {
		log.Println("Insert error:", err)
	}
	return err
}

func convertCurrencyHandler(w http.ResponseWriter, r *http.Request) {
	var req ConvertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	countryCode := strings.ToUpper(req.CountryCode)

	currencyCode, currencyName, err := GetCurrencyForCountry(countryCode)
	if err != nil {
		http.Error(w, "Unsupported country code", http.StatusBadRequest)
		return
	}

	convertedAmount, err := ConvertWithAPI(req.Amount, currencyCode)
	if err != nil {
		http.Error(w, "Conversion failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := SaveConversion(req.Amount, convertedAmount, countryCode, currencyCode); err != nil {
		log.Println("DB save error:", err)
	}

	response := ConvertResponse{
		OriginalAmount:  req.Amount,
		ConvertedAmount: convertedAmount,
		CurrencyCode:    currencyCode,
		CurrencyName:    currencyName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

var countryToCurrency = map[string]struct {
	Code string
	Name string
}{
	"US": {"USD", "United States Dollar"},
	"IN": {"INR", "Indian Rupee"},
	"GB": {"GBP", "British Pound"},
	"EU": {"EUR", "Euro"},
	"JP": {"JPY", "Japanese Yen"},
	// Add more as needed
}

func GetCurrencyForCountry(countryCode string) (string, string, error) {
	if val, ok := countryToCurrency[countryCode]; ok {
		return val.Code, val.Name, nil
	}
	return "", "", errors.New("unsupported country")
}

func ConvertWithAPI(amount float64, targetCurrency string) (float64, error) {
	apiURL := fmt.Sprintf("https://api.frankfurter.app/latest?from=USD&to=%s", targetCurrency)

	resp, err := http.Get(apiURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var response struct {
		Rates map[string]float64 `json:"rates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return 0, err
	}

	rate, ok := response.Rates[targetCurrency]
	if !ok {
		return 0, fmt.Errorf("could not find exchange rate for %s", targetCurrency)
	}

	return amount * rate, nil
}
func SaveAsset(name string, content []byte) error {
	_, err := db.Exec(`
        INSERT INTO assets (name, content, created_at)
        VALUES ($1, $2, NOW())
        ON CONFLICT (name) DO NOTHING`, name, content)
	return err
}
func LoadAssets() {
	// List of assets with name and their respective paths
	assets := map[string]string{
		"watermark":    "assets/watermark.png",
		"md_signature": "assets/md_signature.png",
		"logo":         "assets/logo.png",
	}

	for name, path := range assets {
		// Check if the file exists
		if _, err := os.Stat(path); os.IsNotExist(err) {
			log.Printf("Skipping missing asset file: %s", path)
			continue
		}

		// Read the file content as []byte
		fileContent, err := ioutil.ReadFile(path)
		if err != nil {
			log.Printf("Failed to read file %s: %v", path, err)
			continue
		}

		// Save the asset content to the database
		if err := SaveAsset(name, fileContent); err != nil {
			log.Printf("Failed to load %s: %v", name, err)
		} else {
			log.Printf("Loaded asset: %s", name)
		}
	}
}

func GetAsset(name string) ([]byte, error) {
	var content []byte
	err := db.QueryRow(`SELECT content FROM assets WHERE name = $1`, name).Scan(&content)
	return content, err
}

func uploadAssetHandler(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20) // 10 MB
	if err != nil {
		http.Error(w, "Unable to parse form", http.StatusBadRequest)
		return
	}

	expectedFields := []string{"watermark", "md_signature", "logo"}

	for _, field := range expectedFields {
		file, _, err := r.FormFile(field)
		if err != nil {
			log.Printf("Error retrieving field %s: %v", field, err)
			continue // Don't break, try to get others
		}
		defer file.Close()

		fileContent, err := ioutil.ReadAll(file)
		if err != nil {
			log.Printf("Error reading file %s: %v", field, err)
			continue
		}

		err = SaveAsset(field, fileContent)
		if err != nil {
			log.Printf("Error saving asset %s: %v", field, err)
			continue
		}
		log.Printf("Saved asset: %s", field)
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Files uploaded successfully"))
}

func getLatestAssetsHandler(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query(`
		SELECT name, content 
		FROM assets 
		WHERE name IN ('logo', 'watermark', 'md_signature') 
		ORDER BY created_at DESC
	`)
	if err != nil {
		http.Error(w, "Error fetching assets", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	assets := make(map[string]string)
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.Name, &a.Content); err != nil {
			http.Error(w, "Scan failed", http.StatusInternalServerError)
			return
		}
		// Convert binary data to base64
		assets[a.Name] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(a.Content)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assets)
}

func main() {
	InitDB()

	r := mux.NewRouter()
	r.HandleFunc("/api/currency", convertCurrencyHandler).Methods("POST")
	r.HandleFunc("/api/upload-asset", uploadAssetHandler).Methods("POST") // Add this line
	r.HandleFunc("/api/upload-asset", getLatestAssetsHandler).Methods("GET")

	log.Println("Server started on port 8081")
	log.Fatal(http.ListenAndServe(":8081", r))
}

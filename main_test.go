package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	originalConfigFileName := configFileName

	// Create a temporary config file for testing
	tempConfig := `{
		"cloudflare_api_token": "test_token",
		"ez_captcha_api_key": "test_ez_key",
		"2captcha_api_key": "test_2captcha_key",
		"recaptcha_site_key": "test_recaptcha_key",
		"email_domain": "test.com",
		"cloudflare_zone_id": "test_zone_id",
		"forward_to_email": "test@example.com",
		"monster_promo_url": "http://test.com/promo",
		"monster_submit_url": "http://test.com/submit",
		"use_proxy": false,
		"use_cloudflare_email": true,
		"debug_mode": true,
		"use_2captcha": false,
		"max_captcha_retries": 3,
		"captcha_timeout": 60
	}`

	tmpfile, err := os.CreateTemp("", "config.*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(tempConfig)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Set the config file name to our temp file
	getConfigFileName := func() string {
		return tmpfile.Name()
	}

	// Modify the loadConfig function to use our test config file
	oldLoadConfig := loadConfig
	loadConfig = func() {
		file, err := os.Open(getConfigFileName())
		if err != nil {
			log.Fatalf("Error opening config file: %v", err)
		}
		defer file.Close()

		decoder := json.NewDecoder(file)
		err = decoder.Decode(&config)
		if err != nil {
			log.Fatalf("Error decoding config file: %v", err)
		}
	}

	// Restore the original loadConfig function after the test
	defer func() {
		loadConfig = oldLoadConfig
		configFileName = originalConfigFileName
	}()

	// Load the config
	loadConfig()

	// Check if the config was loaded correctly
	if config.CloudflareAPIToken != "test_token" {
		t.Errorf("Expected CloudflareAPIToken to be 'test_token', got '%s'", config.CloudflareAPIToken)
	}
	if config.EZCaptchaAPIKey != "test_ez_key" {
		t.Errorf("Expected EZCaptchaAPIKey to be 'test_ez_key', got '%s'", config.EZCaptchaAPIKey)
	}
	if config.MaxCaptchaRetries != 3 {
		t.Errorf("Expected MaxCaptchaRetries to be 3, got %d", config.MaxCaptchaRetries)
	}
}

func TestGenerateRandomAlias(t *testing.T) {
	alias, err := generateRandomAlias(10)
	if err != nil {
		t.Fatalf("generateRandomAlias returned an error: %v", err)
	}
	if len(alias) != 10 {
		t.Errorf("Expected alias length to be 10, got %d", len(alias))
	}
}

func TestCheckCaptchaBalance(t *testing.T) {
	// Mock server to simulate the captcha balance API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("10.5"))
	}))
	defer server.Close()

	// Temporarily override the API URLs
	oldEZCaptchaBaseURL := ezCaptchaBaseURL
	oldTwoCaptchaBaseURL := twoCaptchaBaseURL
	ezCaptchaBaseURL = server.URL
	twoCaptchaBaseURL = server.URL
	defer func() {
		ezCaptchaBaseURL = oldEZCaptchaBaseURL
		twoCaptchaBaseURL = oldTwoCaptchaBaseURL
	}()

	// Test EZ Captcha
	config.UseTwoCaptcha = false
	balance, err := checkCaptchaBalance()
	if err != nil {
		t.Fatalf("checkCaptchaBalance returned an error: %v", err)
	}
	if balance != 10.5 {
		t.Errorf("Expected balance to be 10.5, got %f", balance)
	}

	// Test 2Captcha
	config.UseTwoCaptcha = true
	balance, err = checkCaptchaBalance()
	if err != nil {
		t.Fatalf("checkCaptchaBalance returned an error: %v", err)
	}
	if balance != 10.5 {
		t.Errorf("Expected balance to be 10.5, got %f", balance)
	}
}

func TestCreateCloudflareEmailAlias(t *testing.T) {
	// Mock server to simulate Cloudflare API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"result":  map[string]string{"id": "test_rule_id"},
		})
	}))
	defer server.Close()

	// Temporarily override the Cloudflare API URL
	oldCloudflareAPIBaseURL := cloudflareAPIBaseURL
	cloudflareAPIBaseURL = server.URL
	defer func() {
		cloudflareAPIBaseURL = oldCloudflareAPIBaseURL
	}()

	// Set up test config
	config.CloudflareAPIToken = "test_token"
	config.CloudflareZoneID = "test_zone_id"
	config.EmailDomain = "test.com"
	config.ForwardToEmail = "forward@example.com"

	email, err := createCloudflareEmailAlias()
	if err != nil {
		t.Fatalf("createCloudflareEmailAlias returned an error: %v", err)
	}

	if email == "" {
		t.Error("Expected non-empty email, got empty string")
	}

	if emailDomain := "@" + config.EmailDomain; !strings.HasSuffix(email, emailDomain) {
		t.Errorf("Expected email to end with '%s', got '%s'", emailDomain, email)
	}
}

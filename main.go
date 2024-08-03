package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	CloudflareAPIToken string  `json:"cloudflare_api_token"`
	EZCaptchaAPIKey    string  `json:"ez_captcha_api_key"`
	TwoCaptchaAPIKey   string  `json:"2captcha_api_key"`
	RecaptchaSiteKey   string  `json:"recaptcha_site_key"`
	EmailDomain        string  `json:"email_domain"`
	CloudflareZoneID   string  `json:"cloudflare_zone_id"`
	ForwardToEmail     string  `json:"forward_to_email"`
	MonsterPromoURL    string  `json:"monster_promo_url"`
	MonsterSubmitURL   string  `json:"monster_submit_url"`
	UseProxy           bool    `json:"use_proxy"`
	ProxyUsername      string  `json:"proxy_username"`
	ProxyPassword      string  `json:"proxy_password"`
	ProxyDNS           string  `json:"proxy_dns"`
	ProxyPort          string  `json:"proxy_port"`
	UseCloudflareEmail bool    `json:"use_cloudflare_email"`
	DebugMode          bool    `json:"debug_mode"`
	UseTwoCaptcha      bool    `json:"use_2captcha"`
	MaxCaptchaRetries  int     `json:"max_captcha_retries"`
	CaptchaTimeout     float64 `json:"captcha_timeout"`
}

var config Config

const (
	configFileName       = "config.json"
	cloudflareAPIBaseURL = "https://api.cloudflare.com/client/v4"
	ezCaptchaBaseURL     = "https://api.ez-captcha.com"
	twoCaptchaBaseURL    = "https://api.2captcha.com"
	modeInteractive      = 1
	modeAutomatic        = 2
)

type eZCaptchaTask struct {
	ClientKey string `json:"clientKey"`
	Task      struct {
		Type       string `json:"type"`
		WebsiteURL string `json:"websiteURL"`
		WebsiteKey string `json:"websiteKey"`
		SParams    string `json:"sParams"`
	} `json:"task"`
}

type eZCaptchaResult struct {
	Status   string `json:"status"`
	Solution struct {
		GRecaptchaResponse string `json:"gRecaptchaResponse"`
	} `json:"solution"`
}

type twoCaptchaTask struct {
	ClientKey string `json:"clientKey"`
	Task      struct {
		Type       string `json:"type"`
		WebsiteURL string `json:"websiteURL"`
		WebsiteKey string `json:"websiteKey"`
	} `json:"task"`
}

type twoCaptchaResult struct {
	Status   string `json:"status"`
	Solution struct {
		GRecaptchaResponse string `json:"gRecaptchaResponse"`
	} `json:"solution"`
}

type cloudflareEmailRule struct {
	Actions []struct {
		Type  string   `json:"type"`
		Value []string `json:"value"`
	} `json:"actions"`
	Enabled  bool `json:"enabled"`
	Matchers []struct {
		Field string `json:"field"`
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"matchers"`
	Name     string `json:"name"`
	Priority int    `json:"priority"`
}

func main() {
	loadConfig()
	validateConfig()

	fmt.Println("Welcome to the Call of Duty Monster Energy Promo Bot!")

	balance, err := checkCaptchaBalance()
	if err != nil {
		fmt.Printf("Error checking CAPTCHA balance: %v\n", err)
	} else {
		fmt.Printf("Current CAPTCHA balance: $%.2f\n", balance)
	}

	mode := getUserInput("Select mode (1 for Interactive, 2 for Automatic): ")

	switch mode {
	case "1":
		interactiveMode()
	case "2":
		automaticMode()
	default:
		fmt.Println("Invalid mode selected. Exiting.")
	}
}

func loadConfig() {
	file, err := os.Open(configFileName)
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

func validateConfig() {
	if config.CloudflareAPIToken == "" {
		log.Fatal("Cloudflare API token is missing in the config file")
	}
	if config.EZCaptchaAPIKey == "" && config.TwoCaptchaAPIKey == "" {
		log.Fatal("Both EZ Captcha and 2captcha API keys are missing in the config file")
	}
	if config.RecaptchaSiteKey == "" {
		log.Fatal("ReCaptcha site key is missing in the config file")
	}
	if config.EmailDomain == "" {
		log.Fatal("Email domain is missing in the config file")
	}
	if config.CloudflareZoneID == "" {
		log.Fatal("Cloudflare Zone ID is missing in the config file")
	}
	if config.ForwardToEmail == "" {
		log.Fatal("Forward to email is missing in the config file")
	}
	if config.MonsterPromoURL == "" || config.MonsterSubmitURL == "" {
		log.Fatal("Monster promo URL or submit URL is missing in the config file")
	}
	if config.MaxCaptchaRetries == 0 {
		config.MaxCaptchaRetries = 5 // Set a default value if not specified
	}
	if config.CaptchaTimeout == 0 {
		config.CaptchaTimeout = 120 // Set a default value if not specified
	}
}

func interactiveMode() {
	for {
		fmt.Println("\n--- Starting new entry submission ---")
		if !confirmAction("Continue with submission?") {
			fmt.Println("Exiting interactive mode.")
			return
		}

		err := submitEntry()
		if err != nil {
			fmt.Printf("Error submitting entry: %v\n", err)
		} else {
			fmt.Println("Entry submitted successfully")
		}

		if !confirmAction("Submit another entry?") {
			fmt.Println("Exiting interactive mode.")
			return
		}
	}
}

func automaticMode() {
	delay := getUserInputInt("Enter delay between submissions (in seconds): ")
	fmt.Printf("Running in automatic mode with %d second delay.\n", delay)

	successCount := 0
	totalCount := 0

	for {
		fmt.Println("\n--- Starting new entry submission ---")
		err := submitEntry()
		totalCount++
		if err != nil {
			fmt.Printf("Error submitting entry: %v\n", err)
		} else {
			fmt.Println("Entry submitted successfully")
			successCount++
		}
		fmt.Printf("Success rate: %d/%d (%.2f%%)\n", successCount, totalCount, float64(successCount)/float64(totalCount)*100)
		fmt.Printf("Waiting %d seconds before next submission...\n", delay)
		time.Sleep(time.Duration(delay) * time.Second)
	}
}

func submitEntry() error {
	var email string
	var err error

	if config.UseCloudflareEmail {
		debugPrint("Generating temporary email alias...")
		email, err = createCloudflareEmailAlias()
		if err != nil {
			return fmt.Errorf("error creating email alias: %v", err)
		}
		fmt.Printf("Generated email: %s\n", email)
	} else {
		email = getUserInput("Enter email address: ")
	}

	debugPrint("Solving CAPTCHA...")
	var captchaToken string
	if config.UseTwoCaptcha {
		captchaToken, err = solveCaptchaWith2Captcha()
	} else {
		captchaToken, err = solveCaptchaWithEZCaptcha()
	}
	if err != nil {
		return fmt.Errorf("error solving captcha: %v", err)
	}
	debugPrint("CAPTCHA solved successfully")

	debugPrint("Submitting promo entry...")
	cfClearance, err := submitPromoEntry(email, captchaToken)
	if err != nil {
		return fmt.Errorf("error submitting promo entry: %v", err)
	}

	if cfClearance != "" {
		debugPrint("Cloudflare clearance cookie obtained")
		// Use this cookie for subsequent requests
		// For example, you might want to submit multiple entries:
		for i := 0; i < 5; i++ {
			debugPrint(fmt.Sprintf("Submitting additional entry %d/5", i+1))
			_, err := submitPromoEntryWithCookie(email, captchaToken, cfClearance)
			if err != nil {
				debugPrint(fmt.Sprintf("Error submitting additional entry: %v", err))
			} else {
				debugPrint("Additional entry submitted successfully")
			}
		}
	}

	logSubmission(email)
	return nil
}

func createCloudflareEmailAlias() (string, error) {
	randomAlias, err := generateRandomAlias(10)
	if err != nil {
		return "", fmt.Errorf("error generating random alias: %v", err)
	}

	email := fmt.Sprintf("%s@%s", randomAlias, config.EmailDomain)

	rule := cloudflareEmailRule{
		Actions: []struct {
			Type  string   `json:"type"`
			Value []string `json:"value"`
		}{
			{
				Type:  "forward",
				Value: []string{config.ForwardToEmail},
			},
		},
		Enabled: true,
		Matchers: []struct {
			Field string `json:"field"`
			Type  string `json:"type"`
			Value string `json:"value"`
		}{
			{
				Field: "to",
				Type:  "literal",
				Value: email,
			},
		},
		Name:     fmt.Sprintf("Rule created at %s", time.Now().Format(time.RFC3339)),
		Priority: 0,
	}

	jsonData, err := json.Marshal(rule)
	if err != nil {
		return "", fmt.Errorf("error marshaling JSON: %v", err)
	}

	url := fmt.Sprintf("%s/zones/%s/email/routing/rules", cloudflareAPIBaseURL, config.CloudflareZoneID)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+config.CloudflareAPIToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("error creating email alias, status code: %d, response: %s", resp.StatusCode, string(body))
	}

	return email, nil
}

func generateRandomAlias(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	alias := make([]byte, length)
	for i := range alias {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		alias[i] = charset[n.Int64()]
	}
	return string(alias), nil
}

func solveCaptchaWithEZCaptcha() (string, error) {
	task := eZCaptchaTask{
		ClientKey: config.EZCaptchaAPIKey,
	}
	task.Task.Type = "ReCaptchaV2TaskProxyless"
	task.Task.WebsiteURL = config.MonsterPromoURL
	task.Task.WebsiteKey = config.RecaptchaSiteKey
	task.Task.SParams = `{"id":"0","version":"V2","sitekey":"` + config.RecaptchaSiteKey + `","function":"captchaSubmit","callback":"___grecaptcha_cfg.clients['0']['V']['V']['callback']","pageurl":"` + config.MonsterPromoURL + `"}`

	jsonData, err := json.Marshal(task)
	if err != nil {
		return "", err
	}

	resp, err := http.Post(ezCaptchaBaseURL+"/createTask", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var createTaskResult struct {
		TaskID string `json:"taskId"`
	}
	err = json.NewDecoder(resp.Body).Decode(&createTaskResult)
	if err != nil {
		return "", err
	}

	debugPrint("Waiting for CAPTCHA solution...")
	startTime := time.Now()
	for i := 0; i < config.MaxCaptchaRetries; i++ {
		debugPrint(fmt.Sprintf("Attempt %d/%d: Checking CAPTCHA solution...", i+1, config.MaxCaptchaRetries))
		time.Sleep(10 * time.Second)

		result, err := getEZCaptchaTaskResult(createTaskResult.TaskID)
		if err != nil {
			debugPrint(fmt.Sprintf("Error getting task result: %v", err))
			continue
		}

		if result.Status == "ready" {
			return result.Solution.GRecaptchaResponse, nil
		}

		if time.Since(startTime).Seconds() > config.CaptchaTimeout {
			return "", fmt.Errorf("captcha solving timed out after %.2f seconds", config.CaptchaTimeout)
		}
	}

	return "", fmt.Errorf("captcha solving failed after %d attempts", config.MaxCaptchaRetries)
}

func getEZCaptchaTaskResult(taskID string) (*eZCaptchaResult, error) {
	data := map[string]string{
		"clientKey": config.EZCaptchaAPIKey,
		"taskId":    taskID,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(ezCaptchaBaseURL+"/getTaskResult", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result eZCaptchaResult
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func solveCaptchaWith2Captcha() (string, error) {
	task := twoCaptchaTask{
		ClientKey: config.TwoCaptchaAPIKey,
	}
	task.Task.Type = "ReCaptchaV2TaskProxyless"
	task.Task.WebsiteURL = config.MonsterPromoURL
	task.Task.WebsiteKey = config.RecaptchaSiteKey

	jsonData, err := json.Marshal(task)
	if err != nil {
		return "", err
	}

	resp, err := http.Post(twoCaptchaBaseURL+"/createTask", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var createTaskResult struct {
		TaskID int `json:"taskId"`
	}
	err = json.NewDecoder(resp.Body).Decode(&createTaskResult)
	if err != nil {
		return "", err
	}

	debugPrint("Waiting for CAPTCHA solution...")
	startTime := time.Now()
	for i := 0; i < config.MaxCaptchaRetries; i++ {
		debugPrint(fmt.Sprintf("Attempt %d/%d: Checking CAPTCHA solution...", i+1, config.MaxCaptchaRetries))
		time.Sleep(10 * time.Second)

		result, err := get2CaptchaTaskResult(createTaskResult.TaskID)
		if err != nil {
			debugPrint(fmt.Sprintf("Error getting task result: %v", err))
			continue
		}

		if result.Status == "ready" {
			return result.Solution.GRecaptchaResponse, nil
		}

		if time.Since(startTime).Seconds() > config.CaptchaTimeout {
			return "", fmt.Errorf("captcha solving timed out after %.2f seconds", config.CaptchaTimeout)
		}
	}

	return "", fmt.Errorf("captcha solving failed after %d attempts", config.MaxCaptchaRetries)
}

func get2CaptchaTaskResult(taskID int) (*twoCaptchaResult, error) {
	data := map[string]interface{}{
		"clientKey": config.TwoCaptchaAPIKey,
		"taskId":    taskID,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	resp, err := http.Post(twoCaptchaBaseURL+"/getTaskResult", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result twoCaptchaResult
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func submitPromoEntry(email, captchaToken string) (string, error) {
	data := url.Values{}
	data.Set("Email", email)
	data.Set("g-recaptcha-response", captchaToken)

	var client *http.Client

	if config.UseProxy {
		proxyURL, err := url.Parse(fmt.Sprintf("http://%s:%s@%s:%s", config.ProxyUsername, config.ProxyPassword, config.ProxyDNS, config.ProxyPort))
		if err != nil {
			return "", fmt.Errorf("failed to parse proxy URL: %v", err)
		}

		transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		client = &http.Client{Transport: transport}
	} else {
		client = &http.Client{}
	}

	req, err := http.NewRequest("POST", config.MonsterSubmitURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3")
	req.Header.Add("Cookie", "cookieconsent_status=dismiss")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %v", err)
	}
	debugPrint(fmt.Sprintf("Response from promo submission: %s", string(body)))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("promo submission failed with status code: %d", resp.StatusCode)
	}

	var cfClearance string
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "cf_clearance" {
			cfClearance = cookie.Value
			break
		}
	}

	return cfClearance, nil
}

func submitPromoEntryWithCookie(email, captchaToken, cfClearance string) (string, error) {
	data := url.Values{}
	data.Set("Email", email)
	data.Set("g-recaptcha-response", captchaToken)

	var client *http.Client

	if config.UseProxy {
		proxyURL, err := url.Parse(fmt.Sprintf("http://%s:%s@%s:%s", config.ProxyUsername, config.ProxyPassword, config.ProxyDNS, config.ProxyPort))
		if err != nil {
			return "", fmt.Errorf("failed to parse proxy URL: %v", err)
		}

		transport := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		client = &http.Client{Transport: transport}
	} else {
		client = &http.Client{}
	}

	req, err := http.NewRequest("POST", config.MonsterSubmitURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3")
	req.Header.Add("Cookie", fmt.Sprintf("cookieconsent_status=dismiss; cf_clearance=%s", cfClearance))

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %v", err)
	}
	debugPrint(fmt.Sprintf("Response from additional promo submission: %s", string(body)))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("additional promo submission failed with status code: %d", resp.StatusCode)
	}

	return "", nil
}

func getUserInput(prompt string) string {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func getUserInputInt(prompt string) int {
	for {
		input := getUserInput(prompt)
		value, err := strconv.Atoi(input)
		if err == nil {
			return value
		}
		fmt.Println("Invalid input. Please enter a number.")
	}
}

func confirmAction(prompt string) bool {
	input := getUserInput(fmt.Sprintf("%s (y/n): ", prompt))
	return strings.ToLower(input) == "y"
}

func debugPrint(message string) {
	if config.DebugMode {
		fmt.Printf("[DEBUG] %s\n", message)
	}
}

func logSubmission(email string) {
	logFile, err := os.OpenFile("submissions.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		debugPrint(fmt.Sprintf("Error opening log file: %v", err))
		return
	}
	defer logFile.Close()

	logEntry := fmt.Sprintf("%s - Submitted entry for email: %s\n", time.Now().Format(time.RFC3339), email)
	if _, err := logFile.WriteString(logEntry); err != nil {
		debugPrint(fmt.Sprintf("Error writing to log file: %v", err))
	}
}
func checkCaptchaBalance() (float64, error) {
	var url string

	if config.UseTwoCaptcha {
		url = fmt.Sprintf("https://api.2captcha.com/getBalance?key=%s&action=getbalance", config.TwoCaptchaAPIKey)
	} else {
		url = fmt.Sprintf("https://api.ez-captcha.com/getBalance?clientKey=%s", config.EZCaptchaAPIKey)
	}

	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	balance, err := strconv.ParseFloat(string(body), 64)
	if err != nil {
		return 0, err
	}

	return balance, nil
}

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	gecko "github.com/superoo7/go-gecko/v3"
)

// Global variables
var bot *tgbotapi.BotAPI

// Function to fetch BTC price data
func getBTCPrice() (float64, error) {
	cg := gecko.NewClient(nil)
	price, err := cg.SimpleSinglePrice("bitcoin", "usd")
	if err != nil {
		return 0, err
	}
	return float64(price.MarketPrice), nil
}

// Function to fetch BTC historical market data
func getHistoricalData() (map[string]float64, error) {
	cg := gecko.NewClient(nil)
	data, err := cg.CoinsIDMarketChart("bitcoin", "usd", "365")
	if err != nil {
		return nil, err
	}

	historicalPrices := make(map[string]float64)

	now := time.Now()
	periods := map[string]int{
		"1 Day":    1,
		"7 Days":   7,
		"1 Month":  30,
		"3 Months": 90,
		"6 Months": 180,
		"1 Year":   365,
	}

	for period, days := range periods {
		date := now.AddDate(0, 0, -days).Unix() * 1000 // Convert to milliseconds
		for _, pricePoint := range *data.Prices {
			if int64(pricePoint[0]) >= date {
				historicalPrices[period] = float64(pricePoint[1]) // Convert float32 to float64
				break
			}
		}
	}

	return historicalPrices, nil
}

// Function to fetch BTC current block number
func getBTCBlockNumber() (int64, error) {
	response, err := http.Get("https://mempool.space/api/blocks/tip/height")
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()

	var blockNumber int64
	err = json.NewDecoder(response.Body).Decode(&blockNumber)
	if err != nil {
		return 0, err
	}

	return blockNumber, nil
}

// Function to fetch BTC average transaction fees
func getBTCFees() (float64, float64, float64, error) {
	currentPrice, err := getBTCPrice()
	if err != nil {
		return 0, 0, 0, err
	}

	response, err := http.Get("https://mempool.space/api/v1/fees/recommended")
	if err != nil {
		return 0, 0, 0, err
	}
	defer response.Body.Close()

	var fees struct {
		FastestFee  float64 `json:"fastestFee"`
		HalfHourFee float64 `json:"halfHourFee"`
		HourFee     float64 `json:"hourFee"`
	}
	err = json.NewDecoder(response.Body).Decode(&fees)
	if err != nil {
		return 0, 0, 0, err
	}

	// Convert sat/vB to USD
	toUSD := func(satPerVByte float64) float64 {
		// 1 BTC = 100,000,000 satoshis
		// Transaction size is assumed to be 250 bytes (average)
		return (satPerVByte * 250 * currentPrice) / 100000000
	}

	return toUSD(fees.HourFee), toUSD(fees.HalfHourFee), toUSD(fees.FastestFee), nil
}

// Function to fetch BTC market cap
func getBTCMarketCap() (float64, error) {
	cg := gecko.NewClient(nil)
	coin, err := cg.CoinsID("bitcoin", false, false, false, false, false, false)
	if err != nil {
		return 0, err
	}

	// Extract the market cap in USD
	if coin.MarketData == nil {
		return 0, fmt.Errorf("market data is nil")
	}

	marketCap, ok := coin.MarketData.MarketCap["usd"]
	if !ok {
		return 0, fmt.Errorf("market cap in USD not found")
	}

	return marketCap, nil
}

// Function to fetch BTC hashrate
func getBTCHashrate() (float64, error) {
	response, err := http.Get("https://mempool.space/api/v1/mining/hashrate")
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()

	var hashrate struct {
		TerahashesPerSecond float64 `json:"terahashesPerSecond"`
	}
	err = json.NewDecoder(response.Body).Decode(&hashrate)
	if err != nil {
		return 0, err
	}

	return hashrate.TerahashesPerSecond / 1e6, nil // Convert from TH/s to EH/s
}

// Function to fetch BTC all-time high
func getBTCATH() (float64, error) {
	cg := gecko.NewClient(nil)
	// Get historical data to find ATH
	data, err := cg.CoinsIDMarketChart("bitcoin", "usd", "max")
	if err != nil {
		return 0, fmt.Errorf("error fetching historical data: %v", err)
	}

	if data.Prices == nil || len(*data.Prices) == 0 {
		return 0, fmt.Errorf("no price data available")
	}

	// Find the highest price in the historical data
	var ath float64
	for _, pricePoint := range *data.Prices {
		price := float64(pricePoint[1])
		if price > ath {
			ath = price
		}
	}

	if ath == 0 {
		return 0, fmt.Errorf("could not determine ATH from historical data")
	}

	return ath, nil
}

// Function to send a message
func sendMessage(chatID int64, message string) {
	if bot == nil {
		log.Println("Bot is not initialized")
		return
	}
	log.Println("Sending message:", message)
	msg := tgbotapi.NewMessage(chatID, message)
	_, err := bot.Send(msg)
	if err != nil {
		log.Println("Error sending message:", err)
	}
}

// Handle /btc command
func handleBTCCommand(update tgbotapi.Update) {
	log.Println("Received /btc command")
	currentPrice, err := getBTCPrice()
	if err != nil {
		log.Println("Error fetching BTC price:", err)
		sendMessage(update.Message.Chat.ID, "Error fetching BTC price.")
		return
	}
	message := fmt.Sprintf("Current BTC price: $%.2f", currentPrice)
	sendMessage(update.Message.Chat.ID, message)
}

// Handle /block command
func handleBlockCommand(update tgbotapi.Update) {
	log.Println("Received /block command")
	blockNumber, err := getBTCBlockNumber()
	if err != nil {
		log.Println("Error fetching BTC block number:", err)
		sendMessage(update.Message.Chat.ID, "Error fetching BTC block number.")
		return
	}
	message := fmt.Sprintf("Current BTC block number: %d", blockNumber)
	sendMessage(update.Message.Chat.ID, message)
}

// Handle /fees command
func handleFeesCommand(update tgbotapi.Update) {
	log.Println("Received /fees command")
	low, medium, high, err := getBTCFees()
	if err != nil {
		log.Println("Error fetching BTC fees:", err)
		sendMessage(update.Message.Chat.ID, "Error fetching BTC fees.")
		return
	}
	message := fmt.Sprintf("BTC Transaction Fees:\nLow: $%.2f\nMedium: $%.2f\nHigh: $%.2f", low, medium, high)
	sendMessage(update.Message.Chat.ID, message)
}

// Handle /marketcap command
func handleMarketCapCommand(update tgbotapi.Update) {
	log.Println("Received /marketcap command")
	marketCap, err := getBTCMarketCap()
	if err != nil {
		log.Println("Error fetching BTC market cap:", err)
		sendMessage(update.Message.Chat.ID, "Error fetching BTC market cap.")
		return
	}
	message := fmt.Sprintf("Current BTC market cap: $%.2f", marketCap)
	sendMessage(update.Message.Chat.ID, message)
}

// Handle /hashrate command
func handleHashrateCommand(update tgbotapi.Update) {
	log.Println("Received /hashrate command")
	hashrate, err := getBTCHashrate()
	if err != nil {
		log.Println("Error fetching BTC hashrate:", err)
		sendMessage(update.Message.Chat.ID, "Error fetching BTC hashrate.")
		return
	}
	message := fmt.Sprintf("Current BTC hashrate: %.2f EH/s", hashrate)
	sendMessage(update.Message.Chat.ID, message)
}

// Handle /change command
func handleChangeCommand(update tgbotapi.Update) {
	log.Println("Received /change command")
	currentPrice, err := getBTCPrice()
	if err != nil {
		log.Println("Error fetching current BTC price:", err)
		sendMessage(update.Message.Chat.ID, "Error fetching current BTC price.")
		return
	}

	historicalData, err := getHistoricalData()
	if err != nil {
		log.Println("Error fetching historical data:", err)
		sendMessage(update.Message.Chat.ID, "Error fetching historical data.")
		return
	}

	message := "Percentage changes in BTC price:\n"
	for period, historicalPrice := range historicalData {
		change := ((currentPrice - historicalPrice) / historicalPrice) * 100
		message += fmt.Sprintf("%s: %.2f%%\n", period, change)
	}
	sendMessage(update.Message.Chat.ID, message)
}

// Handle /ath command
func handleATHCommand(update tgbotapi.Update) {
	log.Println("Received /ath command")
	ath, err := getBTCATH()
	if err != nil {
		log.Println("Error fetching BTC ATH:", err)
		sendMessage(update.Message.Chat.ID, "Error fetching BTC all-time high.")
		return
	}
	message := fmt.Sprintf("Bitcoin All-Time High: $%.2f", ath)
	sendMessage(update.Message.Chat.ID, message)
}

// HTTP handler for local testing
func handler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello, this is the BTC Bot!"))
}

func main() {
	botToken := os.Getenv("BOT_TOKEN")

	if botToken == "" {
		log.Fatal("BOT_TOKEN environment variable is not set")
	}

	log.Println("BOT_TOKEN:", botToken)

	var err error
	bot, err = tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panic(err)
	}

	log.Println("Bot started and ready to receive commands!")

	// Setting up command handler
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	go func() {
		for update := range updates {
			if update.Message != nil {
				if update.Message.IsCommand() {
					switch update.Message.Command() {
					case "btc":
						handleBTCCommand(update)
					case "block":
						handleBlockCommand(update)
					case "fees":
						handleFeesCommand(update)
					case "marketcap":
						handleMarketCapCommand(update)
					case "hashrate":
						handleHashrateCommand(update)
					case "change":
						handleChangeCommand(update)
					case "ath":
						handleATHCommand(update)
					default:
						log.Println("Unknown command received:", update.Message.Command())
					}
				}
			}
		}
	}()

	// HTTP server for local testing
	http.HandleFunc("/", handler)
	log.Println("Starting server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

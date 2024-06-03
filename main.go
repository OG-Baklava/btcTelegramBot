package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	gecko "github.com/superoo7/go-gecko/v3"
)

// Global variables
var bot *tgbotapi.BotAPI
var chatID int64

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

// Function to send a message
func sendMessage(chatID int64, message string) {
	log.Println("Sending message:", message)
	msg := tgbotapi.NewMessage(chatID, message)
	_, err := bot.Send(msg)
	if err != nil {
		log.Println("Error sending message:", err)
	}
}

// Handle /btc command
func handleBTCCommand(update tgbotapi.Update) {
	currentPrice, err := getBTCPrice()
	if err != nil {
		log.Println("Error fetching BTC price:", err)
		sendMessage(update.Message.Chat.ID, "Error fetching BTC price.")
		return
	}
	message := fmt.Sprintf("Current BTC price: $%.2f", currentPrice)
	sendMessage(update.Message.Chat.ID, message)
}

// Handle /change command
func handleChangeCommand(update tgbotapi.Update) {
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

// HTTP handler for local testing
func handler(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Hello, this is the BTC Bot!"))
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	botToken := os.Getenv("BOT_TOKEN")
	chatIDStr := os.Getenv("CHAT_ID")

	log.Println("BOT_TOKEN:", botToken)
	log.Println("CHAT_ID:", chatIDStr)

	chatID, err = strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		log.Fatal("Error parsing CHAT_ID")
	}

	bot, err = tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panic(err)
	}

	sendMessage(chatID, "Bot started and ready to receive commands!")

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
					case "change":
						handleChangeCommand(update)
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

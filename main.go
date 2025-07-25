package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	gecko "github.com/superoo7/go-gecko/v3"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
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
	// Define periods in order from shortest to longest
	periods := []struct {
		label string
		days  int
	}{
		{"1 Day", 1},
		{"7 Days", 7},
		{"1 Month", 30},
		{"3 Months", 90},
		{"6 Months", 180},
		{"1 Year", 365},
	}

	for _, period := range periods {
		date := now.AddDate(0, 0, -period.days).Unix() * 1000 // Convert to milliseconds
		for _, pricePoint := range *data.Prices {
			if int64(pricePoint[0]) >= date {
				historicalPrices[period.label] = float64(pricePoint[1]) // Convert float32 to float64
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
	// Use CoinGecko simple price endpoint for market cap
	response, err := http.Get("https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd&include_market_cap=true")
	if err != nil {
		return 0, fmt.Errorf("error fetching market cap: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API returned non-200 status code: %d", response.StatusCode)
	}

	var data struct {
		Bitcoin struct {
			MarketCap float64 `json:"usd_market_cap"`
		} `json:"bitcoin"`
	}

	err = json.NewDecoder(response.Body).Decode(&data)
	if err != nil {
		return 0, fmt.Errorf("error parsing market cap data: %v", err)
	}

	return data.Bitcoin.MarketCap, nil
}

// Function to fetch BTC hashrate
func getBTCHashrate() (float64, error) {
	resp, err := http.Get("https://api.blockchain.info/stats")
	if err != nil {
		return 0, fmt.Errorf("error fetching hashrate: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API returned non-200 status code: %d", resp.StatusCode)
	}

	var data struct {
		Hashrate float64 `json:"hash_rate"`
	}
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return 0, fmt.Errorf("error parsing hashrate data: %v", err)
	}

	// Convert from GH/s to EH/s (1 EH/s = 1,000,000,000 GH/s)
	return data.Hashrate / 1e9, nil
}

// Function to fetch BTC all-time high
func getBTCATH() (float64, error) {
	cg := gecko.NewClient(nil)
	// Use CoinsID with specific parameters to get ATH data
	coin, err := cg.CoinsID("bitcoin", false, false, true, false, false, false)
	if err != nil {
		return 0, fmt.Errorf("error fetching coin data: %v", err)
	}

	if coin.MarketData == nil {
		return 0, fmt.Errorf("market data is nil")
	}

	ath, ok := coin.MarketData.ATH["usd"]
	if !ok {
		return 0, fmt.Errorf("ATH data not found")
	}

	// Get the date of ATH
	athDate, ok := coin.MarketData.ATHDate["usd"]
	if !ok {
		return ath, nil // Return ATH even if we can't get the date
	}

	// Parse the date string
	date, err := time.Parse(time.RFC3339, athDate)
	if err != nil {
		log.Printf("Error parsing ATH date: %v", err)
		return ath, nil // Return ATH even if we can't parse the date
	}

	// Format the message to include the date
	message := fmt.Sprintf("Bitcoin All-Time High: $%.2f (reached on %s)", ath, date.Format("January 2, 2006"))
	log.Println(message) // Log the full message for debugging

	return ath, nil
}

// Function to fetch BTC 24-hour trading volume
func getBTCVolume() (float64, error) {
	response, err := http.Get("https://api.coingecko.com/api/v3/simple/price?ids=bitcoin&vs_currencies=usd&include_24hr_vol=true")
	if err != nil {
		return 0, fmt.Errorf("error fetching volume: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API returned non-200 status code: %d", response.StatusCode)
	}

	var data struct {
		Bitcoin struct {
			Volume24h float64 `json:"usd_24h_vol"`
		} `json:"bitcoin"`
	}

	err = json.NewDecoder(response.Body).Decode(&data)
	if err != nil {
		return 0, fmt.Errorf("error parsing volume data: %v", err)
	}

	return data.Bitcoin.Volume24h, nil
}

// Function to fetch the Fear & Greed Index
func getFearGreedIndex() (int, error) {
	response, err := http.Get("https://api.alternative.me/fng/")
	if err != nil {
		return 0, fmt.Errorf("error fetching Fear & Greed Index: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("API returned non-200 status code: %d", response.StatusCode)
	}

	var data struct {
		Data []struct {
			Value string `json:"value"`
		} `json:"data"`
	}

	err = json.NewDecoder(response.Body).Decode(&data)
	if err != nil {
		return 0, fmt.Errorf("error parsing Fear & Greed Index data: %v", err)
	}

	if len(data.Data) == 0 {
		return 0, fmt.Errorf("no data returned from Fear & Greed Index API")
	}

	value, err := strconv.Atoi(data.Data[0].Value)
	if err != nil {
		return 0, fmt.Errorf("error converting Fear & Greed Index value to integer: %v", err)
	}

	return value, nil
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
	message := fmt.Sprintf("Current BTC market cap: $%s", formatNumber(marketCap))
	sendMessage(update.Message.Chat.ID, message)
}

// Helper function to format large numbers with commas
func formatNumber(num float64) string {
	p := message.NewPrinter(language.English)
	return p.Sprintf("%0.2f", num)
}

// Helper function to format large numbers in a readable way
func formatLargeNumber(num float64) string {
	if num >= 1e12 {
		return fmt.Sprintf("%.1fT", num/1e12)
	} else if num >= 1e9 {
		return fmt.Sprintf("%.1fB", num/1e9)
	} else if num >= 1e6 {
		return fmt.Sprintf("%.1fM", num/1e6)
	} else if num >= 1e3 {
		return fmt.Sprintf("%.1fK", num/1e3)
	}
	return fmt.Sprintf("%.0f", num)
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

	// Define the order of periods
	periods := []string{
		"1 Day",
		"7 Days",
		"1 Month",
		"3 Months",
		"6 Months",
		"1 Year",
	}

	message := "Percentage changes in BTC price:\n"
	for _, period := range periods {
		if historicalPrice, ok := historicalData[period]; ok {
			change := ((currentPrice - historicalPrice) / historicalPrice) * 100
			message += fmt.Sprintf("%s: %.2f%%\n", period, change)
		}
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

	// Get the date of ATH
	coin, err := gecko.NewClient(nil).CoinsID("bitcoin", false, false, true, false, false, false)
	if err != nil {
		sendMessage(update.Message.Chat.ID, fmt.Sprintf("Bitcoin All-Time High: $%.2f", ath))
		return
	}

	athDate, ok := coin.MarketData.ATHDate["usd"]
	if !ok {
		sendMessage(update.Message.Chat.ID, fmt.Sprintf("Bitcoin All-Time High: $%.2f", ath))
		return
	}

	date, err := time.Parse(time.RFC3339, athDate)
	if err != nil {
		sendMessage(update.Message.Chat.ID, fmt.Sprintf("Bitcoin All-Time High: $%.2f", ath))
		return
	}

	message := fmt.Sprintf("Bitcoin All-Time High: $%.2f (reached on %s)", ath, date.Format("January 2, 2006"))
	sendMessage(update.Message.Chat.ID, message)
}

// Handle /volume command
func handleVolumeCommand(update tgbotapi.Update) {
	log.Println("Received /volume command")
	volume, err := getBTCVolume()
	if err != nil {
		log.Println("Error fetching BTC volume:", err)
		sendMessage(update.Message.Chat.ID, "Error fetching BTC 24-hour trading volume.")
		return
	}
	message := fmt.Sprintf("BTC 24-hour trading volume: $%s", formatNumber(volume))
	sendMessage(update.Message.Chat.ID, message)
}

// Handle /feargreed command
func handleFearGreedCommand(update tgbotapi.Update) {
	log.Println("Received /feargreed command")
	index, err := getFearGreedIndex()
	if err != nil {
		log.Println("Error fetching Fear & Greed Index:", err)
		sendMessage(update.Message.Chat.ID, "Error fetching Fear & Greed Index.")
		return
	}

	// Fetch the Fear & Greed Index image
	imageURL := fmt.Sprintf("https://alternative.me/crypto/fear-and-greed-index.png")
	response, err := http.Get(imageURL)
	if err != nil {
		log.Println("Error fetching Fear & Greed Index image:", err)
		sendMessage(update.Message.Chat.ID, "Error fetching Fear & Greed Index image.")
		return
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		log.Println("Error fetching Fear & Greed Index image: non-200 status code")
		sendMessage(update.Message.Chat.ID, "Error fetching Fear & Greed Index image.")
		return
	}

	// Create a new photo message
	photo := tgbotapi.NewPhoto(update.Message.Chat.ID, tgbotapi.FileReader{
		Name:   "fear_and_greed.png",
		Reader: response.Body,
	})
	photo.Caption = fmt.Sprintf("Current Fear & Greed Index: %d", index)

	// Send the photo
	_, err = bot.Send(photo)
	if err != nil {
		log.Println("Error sending Fear & Greed Index image:", err)
		sendMessage(update.Message.Chat.ID, "Error sending Fear & Greed Index image.")
		return
	}
}

// Function to scrape assets from the website
func scrapeAssetsFromWebsite() ([]struct {
	Rank      int
	Name      string
	Symbol    string
	MarketCap float64
}, error) {
	response, err := http.Get("https://companiesmarketcap.com/assets-by-market-cap/")
	if err != nil {
		return nil, fmt.Errorf("error fetching website: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("website returned non-200 status code: %d", response.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		return nil, fmt.Errorf("error parsing HTML: %v", err)
	}

	var assets []struct {
		Rank      int
		Name      string
		Symbol    string
		MarketCap float64
	}

	// Try different table selectors
	selectors := []string{
		"table tbody tr",
		"table tr",
		".table tbody tr",
		"#assets-table tbody tr",
		"[data-testid='assets-table'] tbody tr",
	}

	var foundRows bool
	for _, selector := range selectors {
		rows := doc.Find(selector)
		log.Printf("Found %d rows with selector: %s", rows.Length(), selector)

		if rows.Length() > 0 {
			foundRows = true
			rows.Each(func(i int, s *goquery.Selection) {
				if i >= 10 {
					return
				}

				cells := s.Find("td")
				log.Printf("Row %d has %d cells", i, cells.Length())

				if cells.Length() < 3 {
					return
				}

				rankText := strings.TrimSpace(cells.Eq(0).Text())
				nameText := strings.TrimSpace(cells.Eq(1).Text())
				marketCapText := strings.TrimSpace(cells.Eq(2).Text())

				log.Printf("Row %d: Rank='%s', Name='%s', MarketCap='%s'", i, rankText, nameText, marketCapText)

				// Parse rank
				rank, err := strconv.Atoi(rankText)
				if err != nil {
					log.Printf("Error parsing rank '%s': %v", rankText, err)
					return
				}

				// Parse market cap (remove $ and T/B/M suffixes)
				marketCapText = strings.ReplaceAll(marketCapText, "$", "")
				marketCapText = strings.ReplaceAll(marketCapText, ",", "")

				var multiplier float64 = 1
				if strings.HasSuffix(marketCapText, "T") {
					multiplier = 1e12
					marketCapText = strings.TrimSuffix(marketCapText, "T")
				} else if strings.HasSuffix(marketCapText, "B") {
					multiplier = 1e9
					marketCapText = strings.TrimSuffix(marketCapText, "B")
				} else if strings.HasSuffix(marketCapText, "M") {
					multiplier = 1e6
					marketCapText = strings.TrimSuffix(marketCapText, "M")
				}

				// Trim any remaining whitespace
				marketCapText = strings.TrimSpace(marketCapText)

				marketCap, err := strconv.ParseFloat(marketCapText, 64)
				if err != nil {
					log.Printf("Error parsing market cap '%s': %v", marketCapText, err)
					return
				}

				// Extract name and symbol - handle multi-line format
				name := nameText
				symbol := ""

				// Split by newlines and clean up
				lines := strings.Split(nameText, "\n")
				if len(lines) >= 2 {
					name = strings.TrimSpace(lines[0])
					symbol = strings.TrimSpace(lines[1])
				} else if len(lines) == 1 {
					// Fallback to space-based parsing
					if idx := strings.LastIndex(nameText, " "); idx != -1 {
						name = strings.TrimSpace(nameText[:idx])
						symbol = strings.TrimSpace(nameText[idx+1:])
					}
				}

				assets = append(assets, struct {
					Rank      int
					Name      string
					Symbol    string
					MarketCap float64
				}{
					Rank:      rank,
					Name:      name,
					Symbol:    symbol,
					MarketCap: marketCap * multiplier,
				})
			})
			break
		}
	}

	if !foundRows {
		log.Println("No table rows found with any selector")
		// Log the page title and some content for debugging
		title := doc.Find("title").Text()
		log.Printf("Page title: %s", title)

		// Try to find any table
		tables := doc.Find("table")
		log.Printf("Found %d tables on the page", tables.Length())

		// Log first few lines of HTML for debugging
		html, _ := doc.Html()
		if len(html) > 500 {
			log.Printf("First 500 chars of HTML: %s", html[:500])
		}
	}

	log.Printf("Scraped %d assets", len(assets))
	return assets, nil
}

// Handle /assets command
func handleAssetsCommand(update tgbotapi.Update) {
	log.Println("Received /assets command")

	// Try API first
	response, err := http.Get("https://companiesmarketcap.com/api/assets/")
	if err != nil {
		log.Println("API failed, trying web scraping...")
		assets, err := scrapeAssetsFromWebsite()
		if err != nil {
			log.Println("Error scraping assets:", err)
			sendMessage(update.Message.Chat.ID, "Error fetching assets list.")
			return
		}
		displayAssets(update, assets)
		return
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		log.Println("API returned non-200, trying web scraping...")
		assets, err := scrapeAssetsFromWebsite()
		if err != nil {
			log.Println("Error scraping assets:", err)
			sendMessage(update.Message.Chat.ID, "Error fetching assets list.")
			return
		}
		displayAssets(update, assets)
		return
	}

	var data struct {
		Assets []struct {
			Rank      int     `json:"rank"`
			Name      string  `json:"name"`
			Symbol    string  `json:"symbol"`
			MarketCap float64 `json:"marketCap"`
		} `json:"assets"`
	}

	err = json.NewDecoder(response.Body).Decode(&data)
	if err != nil {
		log.Println("Error parsing API data, trying web scraping...")
		assets, err := scrapeAssetsFromWebsite()
		if err != nil {
			log.Println("Error scraping assets:", err)
			sendMessage(update.Message.Chat.ID, "Error fetching assets list.")
			return
		}
		displayAssets(update, assets)
		return
	}

	// Convert API data to common format
	var assets []struct {
		Rank      int
		Name      string
		Symbol    string
		MarketCap float64
	}
	for i, asset := range data.Assets {
		if i >= 10 {
			break
		}
		assets = append(assets, struct {
			Rank      int
			Name      string
			Symbol    string
			MarketCap float64
		}{
			Rank:      asset.Rank,
			Name:      asset.Name,
			Symbol:    asset.Symbol,
			MarketCap: asset.MarketCap,
		})
	}
	displayAssets(update, assets)
}

// Helper function to display assets
func displayAssets(update tgbotapi.Update, assets []struct {
	Rank      int
	Name      string
	Symbol    string
	MarketCap float64
}) {
	message := "🏆 Top 10 Assets by Market Cap\n\n"
	for _, asset := range assets {
		// Check if this is Bitcoin and format accordingly
		if strings.ToLower(asset.Name) == "bitcoin" || strings.ToLower(asset.Symbol) == "btc" {
			message += fmt.Sprintf("%2d. 🟡 **%s** (%s)\n   $%s\n",
				asset.Rank,
				asset.Name,
				asset.Symbol,
				formatLargeNumber(asset.MarketCap))
		} else {
			message += fmt.Sprintf("%2d. %s (%s)\n   $%s\n",
				asset.Rank,
				asset.Name,
				asset.Symbol,
				formatLargeNumber(asset.MarketCap))
		}
	}
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
					case "volume":
						handleVolumeCommand(update)
					case "feargreed":
						handleFearGreedCommand(update)
					case "assets":
						handleAssetsCommand(update)
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

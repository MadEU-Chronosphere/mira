// config/initwa.go
package config

import (
	"chronosphere/utils"
	"context"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
	"github.com/mdp/qrterminal"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// func eventHandler(evt interface{}) {
// 	switch v := evt.(type) {
// 	case *events.Message:
// 		fmt.Println("Received a message!", v.Message.GetConversation())
// 	}
// }

func InitWA(dbAddress string) (*whatsmeow.Client, context.Context, error) {
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	ctx := context.Background()
	container, err := sqlstore.New(ctx, "postgres", dbAddress, dbLog)
	if err != nil {
		log.Fatal("Failed to initialize ", utils.ColorText("Whatsapp ", utils.Red), "client, error: ", err)
		return nil, nil, fmt.Errorf("failed to create sqlstore: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		log.Fatal("Failed to get ", utils.ColorText("Device first ", utils.Yellow), ", error: ", err)
		return nil, nil, fmt.Errorf("failed to get device: %w", err)
	}

	clientLog := waLog.Stdout("Client", "INFO", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	// client.AddEventHandler(eventHandler)

	if client.Store.ID == nil {
		qrChan, _ := client.GetQRChannel(ctx)
		if err := client.Connect(); err != nil {
			log.Fatal("Failed to connect ", utils.ColorText("Whatsapp ", utils.Red), "client, error: ", err)
			return nil, nil, fmt.Errorf("failed to connect client: %w", err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				// Create email body with instructions
				emailBody := `Please scan the attached QR code with WhatsApp to authenticate your MadEU Notification account.

Instructions:
1. Open WhatsApp on your mobile phone
2. Tap the Menu (⋮) or Settings icon
3. Select "Linked Devices" or "WhatsApp Web"
4. Tap on "Link a Device"
5. Scan the QR code attached to this email

Note: This QR code will expire in a short time. If it expires, you'll receive a new one automatically.`

				// Send email with QR code attachment
				err := utils.SendQRCodeEmail(
					os.Getenv("QR_CODE_EMAIL_RECEIVER"),
					"MadEU Notification - WhatsApp Authentication QR Code",
					emailBody,
					evt.Code,
				)

				if err != nil {
					log.Printf("Failed to send QR code email: %v", err)
					// Fallback to plain text email
					utils.SendEmail(
						os.Getenv("QR_CODE_EMAIL_RECEIVER"),
						"MadEU Notification - WhatsApp QR Code",
						"Scan this QR code with WhatsApp: "+evt.Code,
					)
				}

				fmt.Println("QR code has been sent to your email!")
				fmt.Println("Scan this QR code with WhatsApp (also shown in terminal):")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		if err := client.Connect(); err != nil {
			log.Fatal("Failed to connect ", utils.ColorText("Whatsapp ", utils.Red), "client, error: ", err)
			return nil, nil, fmt.Errorf("failed to connect client: %w", err)
		}
	}

	log.Print("✅ Connected to ", utils.ColorText("Whatsapp", utils.Green), " successfully")
	return client, ctx, nil
}

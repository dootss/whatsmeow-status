package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    _ "github.com/mattn/go-sqlite3"
    "github.com/mdp/qrterminal/v3"

    "go.mau.fi/whatsmeow"
    waProto "go.mau.fi/whatsmeow/binary/proto"
    "go.mau.fi/whatsmeow/store/sqlstore"
    "go.mau.fi/whatsmeow/types"
    "go.mau.fi/whatsmeow/types/events"
    waLog "go.mau.fi/whatsmeow/util/log"
    "google.golang.org/protobuf/proto"
)

func GetEventHandler(client *whatsmeow.Client) func(interface{}) {
    return func(evt interface{}) {
        switch v := evt.(type) {
        case *events.Message:
            // Ensure the message is a text message
            if v.Info.Type != "text" {
                return
            }

            // Retrieve the client's own phone number from the client's JID
            ownNumber := client.Store.ID.User

            // Parse the message text
            var messageBody string
            if v.Message.GetExtendedTextMessage() != nil {
                messageBody = v.Message.GetExtendedTextMessage().GetText()
            } else {
                messageBody = v.Message.GetConversation()
            }

            // Check if the message starts with "!status"
            if len(messageBody) > 7 && messageBody[:7] == "!status" {
                var reaction *waProto.Message
                if v.Info.Sender.User == ownNumber {
                    // Extract the status message
                    statusText := messageBody[8:]

                    // Construct the status update message
                    statusMessage := &waProto.Message{
                        ExtendedTextMessage: &waProto.ExtendedTextMessage{
                            Text:           proto.String(statusText),
                            BackgroundArgb: proto.Uint32(0xFF000000), // Example ARGB color (black)
                            TextArgb:       proto.Uint32(0xFFFFFFFF), // Example ARGB color (white)
                            Font:           waProto.ExtendedTextMessage_SYSTEM.Enum(), // Example font
                        },
                    }

                    // Use types.StatusBroadcastJID for posting status updates
                    _, err := client.SendMessage(context.Background(), types.StatusBroadcastJID, statusMessage)
                    if err != nil {
                        fmt.Printf("Failed to post status update: %v\n", err)
                        reaction = client.BuildReaction(v.Info.Chat, v.Info.Sender, v.Info.ID, "❌")
                    } else {
                        fmt.Println("Status update posted successfully.")
                        reaction = client.BuildReaction(v.Info.Chat, v.Info.Sender, v.Info.ID, "✅")
                    }
                } else {
                    // If the sender is not the authenticated user, react with a cross
                    reaction = client.BuildReaction(v.Info.Chat, v.Info.Sender, v.Info.ID, "❌")
                }

                // Send the reaction
                if reaction != nil {
                    _, err := client.SendMessage(context.Background(), v.Info.Chat, reaction)
                    if err != nil {
                        fmt.Printf("Failed to send reaction: %v\n", err)
                    }
                }
            }
        }
    }
}

func main() {
    dbLog := waLog.Stdout("Database", "DEBUG", true)
    container, err := sqlstore.New("sqlite3", "file:examplestore.db?_foreign_keys=on", dbLog)
    if err != nil {
        panic(err)
    }

    deviceStore, err := container.GetFirstDevice()
    if err != nil {
        panic(err)
    }

    clientLog := waLog.Stdout("Client", "INFO", true)
    client := whatsmeow.NewClient(deviceStore, clientLog)
    client.AddEventHandler(GetEventHandler(client))

    client.DontSendSelfBroadcast = false

    if client.Store.ID == nil {
        qrChan, _ := client.GetQRChannel(context.Background())
        err = client.Connect()
        if err != nil {
            panic(err)
        }
        for evt := range qrChan {
            if evt.Event == "code" {
                qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
            } else {
                fmt.Println("Login event:", evt.Event)
            }
        }
    } else {
        err = client.Connect()
        if err != nil {
            panic(err)
        }
        // Update presence status
        err = client.SendPresence(types.PresenceAvailable)
        if err != nil {
            fmt.Printf("Failed to update presence: %v\n", err)
        }
    
        // Check status privacy settings
        statusPrivacy, err := client.GetStatusPrivacy()
        if err != nil {
            fmt.Printf("Failed to get status privacy settings: %v\n", err)
        } else {
            fmt.Println("Status Privacy Settings:", statusPrivacy)
        }
    }

    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    <-c

    client.Disconnect()
}

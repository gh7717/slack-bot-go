package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nlopes/slack"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type User struct {
	Info   slack.User
	Rating int
}

type Token struct {
	Token string `json:"token"`
}

type Message struct {
	ChannelId string
	Timestamp string
	Payload   string
	Rating    int
	User      User
}

type BotCentral struct {
	Group  *slack.Group
	Event  *slack.MessageEvent
	UserId string
}

type AttachmentChannel struct {
	Attachment   *slack.Attachment
	DisplayTitle string
}

type Messages []Message

func (u Messages) Len() int {
	return len(u)
}
func (u Messages) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]
}
func (u Messages) Less(i, j int) bool {
	return u[i].Rating > u[j].Rating
}

type ActiveUsers []User

func (u ActiveUsers) GetMeanRating() string {
	var sum float64
	length := u.Len()
	for i := 0; i < length; i++ {
		sum += float64(u[i].Rating)
	}
	return fmt.Sprintf("%6.3f", sum/float64(length))
}

func (u ActiveUsers) FindUser(ID string) User {
	for i := 0; i < u.Len(); i++ {
		if u[i].Info.ID == ID || u[i].Info.Name == ID || u[i].Info.RealName == ID {
			return u[i]
		}
	}
	return User{}
}

func (u ActiveUsers) Len() int {
	return len(u)
}
func (u ActiveUsers) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]
}
func (u ActiveUsers) Less(i, j int) bool {
	return u[i].Rating > u[j].Rating
}

const TOP = 20

var (
	api               *slack.Client
	botKey            Token
	userId            string
	activeUsers       ActiveUsers
	userMessages      Messages
	botId             string
	botCommandChannel chan *BotCentral
	botReplyChannel   chan AttachmentChannel
	API               string
	channelId         string
)

func init() {
	file, err := ioutil.ReadFile("./token.json")

	if err != nil {
		log.Fatal("File doesn't exist")
	}

	if err := json.Unmarshal(file, &botKey); err != nil {
		log.Fatal("Cannot parse token.json")
	}
	userId = os.Getenv("USER")
	channelId = os.Getenv("CHANNEL")

	API = os.Getenv("API")
}

func handleBotCommands(c chan AttachmentChannel) {
	commands := map[string]string{
		"help":      "will tell you the available bot commands.",
		"current":   "will tell you who is currently awaiting a ticket.",
		"order":     "will tell you the list of engineer users in current team.",
		"next":      "will advance the rotation to the next person. After a ticket comes in and is assigned to the current person it is their responsibility to advance the rotation by using the *next* command. This can also be done by an Admin if that person is not available.",
		"blacklist": "Admin only command that removes a person from the rotation until they are whitelisted again. It is important to only blacklist 1 person at a time and use their ‘@’ handle in Slack. If multiple people need to be blacklisted issue the command per person.",
		"whitelist": "Admin only command that returns a person to the rotation that was previously blacklisted. It is important that only 1 person per command be whitelisted at a time.",
		"add":       "Admin only command that adds a new persone to the team list. Admin also is able to add other user as admin.",
		"del":       "Admin only command that deletes a user from team list.",
		"admins":    "shows the list of admin users.",
		"tickets":   "The command shows the list of tickets are opened on current week. Week starts on Sunday and finish on Saturday.",
		"last":      "The command shows the list of tickets are opened on last week. Week starts on Sunday and finish on Saturday.",
		"backlog":   "The command shows the list of the current tickets in our queue."}

	var attachmentChannel AttachmentChannel

	for {
		botChannel := <-botCommandChannel
		log.Println("bot handles a command for bot")
		commandArray := strings.Fields(botChannel.Event.Text)
		log.Print("Handled command: ")
		log.Println(commandArray[1])
		switch commandArray[1] {
		case "help":
			log.Println("Help command")
			fields := make([]slack.AttachmentField, 0)
			for k, v := range commands {
				switch k {
				case "blacklist", "whitelist", "del":
					fields = append(fields, slack.AttachmentField{
						Title: "<bot> " + k + " <user>",
						Value: v,
					})
				case "add":
					fields = append(fields, slack.AttachmentField{
						Title: "<bot> " + k + " <user>" + "<admin| .. >",
						Value: v,
					})
				default:
					fields = append(fields, slack.AttachmentField{
						Title: "<bot> " + k,
						Value: v,
					})
				}
			}
			attachment := &slack.Attachment{
				Pretext: "Command List",
				Color:   "#B733FF",
				Fields:  fields,
			}
			attachmentChannel.Attachment = attachment
			c <- attachmentChannel

		case "current":
			resp, err := http.Get(API + "/current")
			if err != nil {
				log.Println(err)
			}
			fmt.Println("Current user to get ticket.")
			err := json.Unmarshal(resp, user)
			if err != nil {
				log.Println(err)
			}
			fields := make([]slack.AttachmentField, 5)
			for i := 0; i < 5; i++ {
				field := slack.AttachmentField{
					Title: fmt.Sprintf("%v %v from %v", userMessages[i].Rating, "Emojis :smile:", userMessages[i].User.Info.RealName),
					Value: userMessages[i].Payload,
					Short: false,
				}
				fields[i] = field
			}

			attachment := &slack.Attachment{
				Pretext: "Top Messages",
				Color:   "#0a84c1",
				Fields:  fields,
			}
			attachmentChannel.Attachment = attachment
			c <- attachmentChannel

		case "order":
			bottomNumber := commandArray[2]
			if intNumber, err := strconv.Atoi(bottomNumber); err == nil {
				if intNumber > 0 && intNumber <= 20 {
					sort.Sort(activeUsers)
					fields := make([]slack.AttachmentField, intNumber)
					for i := intNumber - 1; i >= 0; i-- {
						field := slack.AttachmentField{
							Title: fmt.Sprintf("%v %v", activeUsers[len(activeUsers)-1-i].Rating, ":star:"),
							Value: fmt.Sprintf("%v", activeUsers[len(activeUsers)-1-i].Info.RealName),
							Short: false,
						}
						fields[i] = field
					}

					attachment := &slack.Attachment{
						Pretext: "Bottom " + fmt.Sprintf("%v", intNumber),
						Color:   "#b01408",
						Fields:  fields,
					}
					attachmentChannel.Attachment = attachment
					c <- attachmentChannel
				}
			}

		case "next":
			log.Println("next")
		case "blacklis":
			log.Println("next")
		case "whitelist":
			log.Println("next")
		case "add":
			log.Println("next")
		case "del":
			log.Println("next")
		case "admins":
			log.Println("next")
		case "tickets":
			log.Println("next")
		case "last":
			log.Println("next")
		case "backlog":
			if len(commandArray) > 2 {
				// mean of
				if len(commandArray) == 4 && commandArray[2] == "of" {
					targetUser := activeUsers.FindUser(commandArray[3])

					attachment := &slack.Attachment{
						Pretext: targetUser.Info.RealName,
						Color:   "#0a84c1",
						Fields: []slack.AttachmentField{{
							Title: "Score",
							Value: fmt.Sprint(targetUser.Rating),
							Short: true,
						}, {
							Title: "Company Mean Score",
							Value: fmt.Sprint(activeUsers.GetMeanRating()),
							Short: true,
						}},
					}

					attachmentChannel.Attachment = attachment
					c <- attachmentChannel
				}
			} else {
				// mean
				user := activeUsers.FindUser(botChannel.UserId)
				attachment := &slack.Attachment{
					Pretext: user.Info.RealName,
					Color:   "#0a84c1",
					Fields: []slack.AttachmentField{{
						Title: "Score",
						Value: fmt.Sprint(user.Rating),
						Short: true,
					}, {
						Title: "Company Mean Score",
						Value: fmt.Sprint(activeUsers.GetMeanRating()),
						Short: true,
					}},
				}

				attachmentChannel.Attachment = attachment
				c <- attachmentChannel
			}
		}
	}
}

func handleBotReply() {
	for {
		ac := <-botReplyChannel
		params := slack.PostMessageParameters{}
		params.Markdown = true
		params.AsUser = true
		params.Attachments = []slack.Attachment{*ac.Attachment}
		log.Printf(channelId)
		_, _, errPostMessage := api.PostMessage(channelId, ac.DisplayTitle, params)
		if errPostMessage != nil {
			log.Println(errPostMessage)
		}
	}
}

func getIpAddress() (string, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	var validTun = regexp.MustCompile(`^.*tun[0-9]+$`)
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue // interface down
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // loopback interface
		}

		if !validTun.MatchString(iface.Name) {
			continue //not a vpn
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return "", err
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue // not an ipv4 address
			}
			return ip.String(), nil
		}
	}
	return "", errors.New("are you connected to the network?")
}

const botRestarted = "The bot has been restarted. \nCurrent ip: "

func main() {
	logger := log.New(os.Stdout, "slack-bot: ", log.Lshortfile|log.LstdFlags)
	slack.SetLogger(logger)

	api = slack.New(botKey.Token)
	api.SetDebug(false)

	rtm := api.NewRTM()

	params := slack.PostMessageParameters{}
	params.Markdown = true
	botCommandChannel = make(chan *BotCentral)
	botReplyChannel = make(chan AttachmentChannel)

	userMessages = make(Messages, 0)

	go rtm.ManageConnection()
	go handleBotCommands(botReplyChannel)
	go handleBotReply()

Loop:
	for {
		select {
		case msg := <-rtm.IncomingEvents:
			switch ev := msg.Data.(type) {
			case *slack.ConnectedEvent:
				botId = ev.Info.User.ID
				log.Println("Infos:", ev.Info)
				log.Println("Connection counter:", ev.ConnectionCount)
				log.Printf("Team ID: %s Team Name: %s Team Domain: %s\n", ev.Info.Team.ID, ev.Info.Team.Name, ev.Info.Team.Domain)
				groups, _ := api.GetGroups(false)
				log.Printf("List of private chats: %v", groups)
				ip, err := getIpAddress()
				if err != nil {
					fmt.Print(err)
				}
				_, _, channel, err := rtm.OpenIMChannel(userId)
				rtm.PostMessage(channel, "```"+botRestarted+ip+"```", params)

			case *slack.MessageEvent:
				log.Println("Channel id:", ev.Channel)
				log.Println("The message has been recieved")
				log.Println(ev.Text)

				switch s := ev.Channel[0]; string(s) {
				case "D":
					log.Println("Direct message")
				case "G":
					log.Println("Group")
					groupInfo, err := api.GetGroupInfo(ev.Channel)
					if err != nil {
						log.Println(err)
					}
					log.Printf("%v", groupInfo)
					botCentral := &BotCentral{
						Group:  groupInfo,
						Event:  ev,
						UserId: ev.User,
					}
					if ev.Type == "message" && strings.HasPrefix(ev.Text, "<@"+botId+">") {
						log.Println("the message for bot")
						botCommandChannel <- botCentral
					}
				case "C":
					log.Println("Channel")
				}
			case *slack.RTMError:
				fmt.Printf("Error: %s\n", ev.Error())

			case *slack.InvalidAuthEvent:
				fmt.Printf("Invalid credentials")
				break Loop

			default:
				// Ignore other events..
				// fmt.Printf("Unexpected: %v\n", msg.Data)
			}
		}
	}
}

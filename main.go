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
	"regexp" //"sort"
	"strconv"
	"strings"
)

type User struct {
	Info   slack.User
	Rating int
}
type UserInfo struct {
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
	RealName string `json:"real_name"`
	Current  bool   `json:"current"`
	IsAdmin  bool   `json:"is_admin"`
	ID       string `json:"id"`
	Engineer bool   `json:"engineer"`
	Attuid   string `json:"attuid"`
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

var (
	api               *slack.Client
	botKey            Token
	userId            string
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
		log.Printf("DEBUG: user %s sent command to the bot", botChannel.UserId)
		resp, err := http.Get(API + "/user/isadmin/" + botChannel.UserId)
		defer resp.Body.Close()
		if err != nil {
			log.Printf("ERROR: get %s/isadmin/%s %s", API, botChannel.Event.Text, err)
		}
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("ERROR: can't read responce: %s", err)
		}
		isadmin, err := strconv.ParseBool(string(body))
		if err != nil {
			log.Printf("ERROR: can't parse bool %s", err)
		}
		log.Printf("DEBUG: Command: %s\n isadmin: %t", commandArray, isadmin)
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
			log.Println("current")
			fields := make([]slack.AttachmentField, 0)
			resp, err := http.Get(API + "/users/current")
			defer resp.Body.Close()
			if err != nil {
				log.Printf("ERROR: get %s/current", API, err)
			}
			log.Println("Current user to get ticket.")
			var user UserInfo
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Printf("ERROR: can't read responce: %s", err)
			}
			log.Printf("DEBUG: User: %s", body)
			err = json.Unmarshal(body, &user)
			if err != nil {
				log.Printf("ERROR: can't parse infromation about the user: %s", err)
			}
			fields = append(fields, slack.AttachmentField{
				Title: "",
				Value: "We are waiting for <@" + user.ID + "> to grab a ticket",
			})
			attachment := &slack.Attachment{
				Pretext: "Current",
				Color:   "#0a84c1",
				Fields:  fields,
			}
			attachmentChannel.Attachment = attachment
			c <- attachmentChannel
		case "order":
			log.Println("order")
			resp, err := http.Get(API + "/users/active")
			defer resp.Body.Close()
			if err != nil {
				log.Printf("ERROR: get %s/current", API, err)
			}
			var active_users []UserInfo
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Printf("ERROR: can't read responce: %s", err)
			}
			log.Printf("DEBUG: Users: %s", body)
			err = json.Unmarshal(body, &active_users)
			if err != nil {
				log.Printf("ERROR: can't parse infromation about the user: %s", err)
			}
			number_of_users := len(active_users)

			fields := make([]slack.AttachmentField, number_of_users)
			for i := 0; i < number_of_users; i++ {
				field := slack.AttachmentField{
					Title: "",
					Value: fmt.Sprintf("<@%s>", active_users[i].ID),
					//Short: false,
				}
				fields[i] = field
			}

			attachment := &slack.Attachment{
				Pretext: "Order of active members in the rotation.",
				Color:   "#0a84c1",
				Fields:  fields,
			}
			attachmentChannel.Attachment = attachment
			c <- attachmentChannel
			resp, err = http.Get(API + "/users/blacklisted")
			defer resp.Body.Close()
			if err != nil {
				log.Printf("ERROR: get %s/blacklisted", API, err)
			}
			var blacklisted_users []UserInfo
			body, err = ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Printf("ERROR: can't read responce: %s", err)
			}
			log.Printf("DEBUG: Users: %s", body)
			err = json.Unmarshal(body, &blacklisted_users)
			if err != nil {
				log.Printf("ERROR: can't parse infromation about the user: %s", err)
			}
			number_of_users = len(blacklisted_users)

			fields = make([]slack.AttachmentField, number_of_users)
			for i := 0; i < number_of_users; i++ {
				field := slack.AttachmentField{
					Title: "",
					Value: fmt.Sprintf("<@%s>", blacklisted_users[i].ID),
					//Short: false,
				}
				fields[i] = field
			}

			attachment = &slack.Attachment{
				Pretext: "Blacklisted members",
				Color:   "#0a84c1",
				Fields:  fields,
			}
			attachmentChannel.Attachment = attachment
			c <- attachmentChannel
		case "next":
			if !isadmin {
				fields := make([]slack.AttachmentField, 0)
				fields = append(fields, slack.AttachmentField{
					Title: "",
					Value: "Only admin can execute next command",
				})
				attachment := &slack.Attachment{
					Pretext: "Current",
					Color:   "#0a84c1",
					Fields:  fields,
				}
				attachmentChannel.Attachment = attachment
				c <- attachmentChannel
			} else {
				log.Println("next")
				fields := make([]slack.AttachmentField, 0)
				resp, err := http.Get(API + "/users/next")
				defer resp.Body.Close()
				if err != nil {
					log.Printf("ERROR: get %s/current", API, err)
				}
				log.Println("Current user to get ticket.")
				var user UserInfo
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					log.Printf("ERROR: can't read responce: %s", err)
				}
				log.Printf("DEBUG: User: %s", body)
				err = json.Unmarshal(body, &user)
				if err != nil {
					log.Printf("ERROR: can't parse infromation about the user: %s", err)
				}
				fields = append(fields, slack.AttachmentField{
					Title: "",
					Value: "We are waiting for <@" + user.ID + "> to grab a ticket",
				})
				attachment := &slack.Attachment{
					Pretext: "Current",
					Color:   "#0a84c1",
					Fields:  fields,
				}
				attachmentChannel.Attachment = attachment
				c <- attachmentChannel
			}
		case "blacklist":
			log.Println("blacklist")
			if !isadmin {
				fields := make([]slack.AttachmentField, 0)
				fields = append(fields, slack.AttachmentField{
					Title: "",
					Value: "Only admin can execute next command",
				})
				attachment := &slack.Attachment{
					Pretext: "Current",
					Color:   "#0a84c1",
					Fields:  fields,
				}
				attachmentChannel.Attachment = attachment
				c <- attachmentChannel
			} else {
				fields := make([]slack.AttachmentField, 0)
				resp, err := http.Get(API + "/users/blacklist/" + userid)
				defer resp.Body.Close()
				if err != nil {
					log.Printf("ERROR: get %s/blacklist/", API, err)
				}
				log.Println("Current user to get ticket.")
				var user UserInfo
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					log.Printf("ERROR: can't read responce: %s", err)
				}
				log.Printf("DEBUG: User: %s", body)
				err = json.Unmarshal(body, &user)
				if err != nil {
					log.Printf("ERROR: can't parse infromation about the user: %s", err)
				}
				fields = append(fields, slack.AttachmentField{
					Title: "",
					Value: "We are waiting for <@" + user.ID + "> to grab a ticket",
				})
				attachment := &slack.Attachment{
					Pretext: "Current",
					Color:   "#0a84c1",
					Fields:  fields,
				}
				attachmentChannel.Attachment = attachment
				c <- attachmentChannel
			}
		case "whitelist":
			log.Println("whitelist")
			if !isadmin {
				fields := make([]slack.AttachmentField, 0)
				fields = append(fields, slack.AttachmentField{
					Title: "",
					Value: "Only admin can execute next command",
				})
				attachment := &slack.Attachment{
					Pretext: "Current",
					Color:   "#0a84c1",
					Fields:  fields,
				}
				attachmentChannel.Attachment = attachment
				c <- attachmentChannel
			} else {
			}
		case "add":
			log.Println("add")
			if !isadmin {
				fields := make([]slack.AttachmentField, 0)
				fields = append(fields, slack.AttachmentField{
					Title: "",
					Value: "Only admin can execute next command",
				})
				attachment := &slack.Attachment{
					Pretext: "Current",
					Color:   "#0a84c1",
					Fields:  fields,
				}
				attachmentChannel.Attachment = attachment
				c <- attachmentChannel
			} else {
			}
		case "del":
			log.Println("del")
			if !isadmin {
				fields := make([]slack.AttachmentField, 0)
				fields = append(fields, slack.AttachmentField{
					Title: "",
					Value: "Only admin can execute next command",
				})
				attachment := &slack.Attachment{
					Pretext: "Current",
					Color:   "#0a84c1",
					Fields:  fields,
				}
				attachmentChannel.Attachment = attachment
				c <- attachmentChannel
			} else {
			}
		case "admins":
			log.Println("admin")
			resp, err := http.Get(API + "/users/admins")
			defer resp.Body.Close()
			if err != nil {
				log.Printf("ERROR: get %s/admins", API, err)
			}
			var active_users []UserInfo
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Printf("ERROR: can't read responce: %s", err)
			}
			log.Printf("DEBUG: Users: %s", body)
			err = json.Unmarshal(body, &active_users)
			if err != nil {
				log.Printf("ERROR: can't parse infromation about the user: %s", err)
			}
			number_of_users := len(active_users)

			fields := make([]slack.AttachmentField, number_of_users)
			for i := 0; i < number_of_users; i++ {
				field := slack.AttachmentField{
					Title: "",
					Value: fmt.Sprintf("<@%s>", active_users[i].ID),
					//Short: false,
				}
				fields[i] = field
			}

			attachment := &slack.Attachment{
				Pretext: "Admins",
				Color:   "#0a84c1",
				Fields:  fields,
			}
			attachmentChannel.Attachment = attachment
			c <- attachmentChannel
		case "tickets":
			log.Println("tickets")
		case "last":
			log.Println("last")
		case "backlog":
			log.Println("backlog")
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
		log.Printf("DEBUG: Channel ID = %s", channelId)
		_, _, errPostMessage := api.PostMessage(channelId, ac.DisplayTitle, params)
		if errPostMessage != nil {
			log.Println("ERROR: error during post message %s", errPostMessage)
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
				//groups, _ := api.GetGroups(false)
				ip, err := getIpAddress()
				if err != nil {
					fmt.Printf("ERROR: getting IP address error: %s", err)
				}
				_, _, channel, err := rtm.OpenIMChannel(userId)
				rtm.PostMessage(channel, "```"+botRestarted+ip+"```", params)

			case *slack.MessageEvent:
				log.Println("Channel id:", ev.Channel)

				switch s := ev.Channel[0]; string(s) {
				case "D":
					log.Println("Direct message")
				case "G":
					log.Println("Group")
					groupInfo, err := api.GetGroupInfo(ev.Channel)
					if err != nil {
						log.Println("ERROR: getting message error %s", err)
					}
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
				log.Printf("Error: %s\n", ev.Error())
			case *slack.LatencyReport:
				log.Printf("Current latency: %v\n", ev.Value)
			case *slack.InvalidAuthEvent:
				log.Printf("Invalid credentials")
				break Loop

			default:
				// Ignore other events..
				// fmt.Printf("Unexpected: %v\n", msg.Data)
			}
		}
	}
}

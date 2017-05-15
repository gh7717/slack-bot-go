package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"regexp" //"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nlopes/slack"
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
	userId            *string
	botId             string
	botCommandChannel chan *BotCentral
	botReplyChannel   chan AttachmentChannel
	API               *string
	channelId         *string
	token             *string
)

func init() {
	userId = flag.String("user", "U03EPQS1F", "slack bot user id")
	channelId = flag.String("channel", "G2VLDKLSX", "slack channel ID")
	API = flag.String("api", "http://localhost:8080/api", "API url")
	token = flag.String("crednetial", "xoxb-88562823922-NYJLdNas6mwYuYiNjLVmPMWf", "slack bot token")
	flag.Parse()

}
func getRequest(uri string) (body []byte, err error) {
	resp, err := http.Get(uri)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil

}

type Reader interface {
	Read(p []byte) (n int, err error)
}

func postRequest(uri string, user UserInfo) (body []byte, err error) {
	buf, err := json.Marshal(user)
	if err != nil {
		return nil, err
	}
	u := bytes.NewReader(buf)
	resp, err := http.Post(uri, "application/json", u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil

}
func permissionDenidedMessage() {

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
		commandArray := strings.Fields(botChannel.Event.Text)
		log.Printf("%v", commandArray)
		resp, err := getRequest(*API + "/user/isadmin/" + botChannel.UserId)
		if err != nil {
			log.Printf("ERROR: is admin check error: %s", err)
		}
		isadmin, err := strconv.ParseBool(string(resp))
		if err != nil {
			log.Printf("ERROR: can't parse bool %s", err)
		}
		switch commandArray[1] {
		case "help": // works
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

		case "current": //works
			log.Println("current")
			fields := make([]slack.AttachmentField, 0)
			resp, err := getRequest(*API + "/users/current")
			if err != nil {
				log.Printf("ERROR: error during current command: %s", resp)
			}
			var user UserInfo

			err = json.Unmarshal(resp, &user)
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
		case "order": //works
			log.Println("order")
			resp, err := getRequest(*API + "/users/active")
			if err != nil {
				log.Printf("ERROR: error during order %s", err)
			}
			var active_users []UserInfo
			err = json.Unmarshal(resp, &active_users)
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
			resp, err = getRequest(*API + "/users/blacklisted")
			if err != nil {
				log.Printf("ERROR: %s/blacklisted", API, err)
			}
			var blacklisted_users []UserInfo
			err = json.Unmarshal(resp, &blacklisted_users)
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
		case "next": // works
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
				resp, err := getRequest(*API + "/users/next")
				if err != nil {
					log.Printf("ERROR: get %s/current", API, err)
				}
				var user UserInfo
				err = json.Unmarshal(resp, &user)
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
		case "blacklist": //works
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
				if len(commandArray) < 3 {
					log.Printf("error: user is not specified")
					fields := make([]slack.AttachmentField, 0)
					fields = append(fields, slack.AttachmentField{
						Title: "",
						Value: "You must specify user",
					})
					attachment := &slack.Attachment{
						Pretext: "Error",
						Color:   "#0a84c1",
						Fields:  fields,
					}
					attachmentChannel.Attachment = attachment
					c <- attachmentChannel
				} else {
					mentionedUser := commandArray[2][2 : len(commandArray[2])-1]
					log.Printf(mentionedUser)
					fields := make([]slack.AttachmentField, 0)
					r, err := getRequest(*API + "/user/blacklist/" + mentionedUser)
					log.Printf("%s", r)
					if err != nil {
						log.Printf("ERROR: get %s/blacklist/", API, err)
					}
					fields = append(fields, slack.AttachmentField{
						Title: "",
						Value: fmt.Sprintf("The user <@%s> has been blacklisted", mentionedUser),
					})
					attachment := &slack.Attachment{
						Pretext: "",
						Color:   "#0a84c1",
						Fields:  fields,
					}
					attachmentChannel.Attachment = attachment
					c <- attachmentChannel
				}
			}
		case "whitelist": //works
			if !isadmin {
				fields := make([]slack.AttachmentField, 0)
				fields = append(fields, slack.AttachmentField{
					Title: "",
					Value: "Only admin can execute whitelist command",
				})
				attachment := &slack.Attachment{
					Pretext: "Current",
					Color:   "#0a84c1",
					Fields:  fields,
				}
				attachmentChannel.Attachment = attachment
				c <- attachmentChannel
			} else {
				if len(commandArray) < 3 {
					log.Printf("error: user is not specified")
					fields := make([]slack.AttachmentField, 0)
					fields = append(fields, slack.AttachmentField{
						Title: "",
						Value: "You must specify user",
					})
					attachment := &slack.Attachment{
						Pretext: "Error",
						Color:   "#0a84c1",
						Fields:  fields,
					}
					attachmentChannel.Attachment = attachment
					c <- attachmentChannel
				} else {
					mentionedUser := commandArray[2][2 : len(commandArray[2])-1]
					log.Printf(mentionedUser)
					fields := make([]slack.AttachmentField, 0)
					r, err := getRequest(*API + "/user/whitelist/" + mentionedUser)
					log.Printf("%s", r)
					if err != nil {
						log.Printf("ERROR: get %s/whitelist/", API, err)
					}
					fields = append(fields, slack.AttachmentField{
						Title: "",
						Value: fmt.Sprintf("The user <@%s> has been whitelisted", mentionedUser),
					})
					attachment := &slack.Attachment{
						Pretext: "",
						Color:   "#0a84c1",
						Fields:  fields,
					}
					attachmentChannel.Attachment = attachment
					c <- attachmentChannel
				}
			}
		case "add":
			if !isadmin {
				fields := make([]slack.AttachmentField, 0)
				fields = append(fields, slack.AttachmentField{
					Title: "",
					Value: "Only admin can execute add command",
				})
				attachment := &slack.Attachment{
					Pretext: "Error",
					Color:   "#0a84c1",
					Fields:  fields,
				}
				attachmentChannel.Attachment = attachment
				c <- attachmentChannel
			} else {
				// check all flags. the flags can be in any order
				if len(commandArray) < 4 {
					log.Printf("error: user is not specified")
					fields := make([]slack.AttachmentField, 0)
					fields = append(fields, slack.AttachmentField{
						Title: "",
						Value: "You must specify user: <bot> add <slack user name> <attuid>",
					})
					attachment := &slack.Attachment{
						Pretext: "Error",
						Color:   "#0a84c1",
						Fields:  fields,
					}
					attachmentChannel.Attachment = attachment
					c <- attachmentChannel
				} else {
					var us UserInfo
					slackUser, err := api.GetUserInfo(commandArray[2][2 : len(commandArray[2])-1])
					if err != nil {
						log.Printf("%s\n", err)
					}
					us.Name = slackUser.Name
					us.IsActive = true
					us.Current = false
					us.RealName = slackUser.RealName
					if len(commandArray) < 5 {
						us.IsAdmin = false
					} else {
						// add parsing admin variable
						us.IsAdmin = true
					}
					us.ID = slackUser.ID
					if len(commandArray) == 6 {
						us.Engineer, err = strconv.ParseBool(commandArray[5])
						if err != nil {
							log.Printf("%s", err)
						}
					} else {
						us.Engineer = true
					}
					us.Attuid = commandArray[3]
					fields := make([]slack.AttachmentField, 0)
					r, err := postRequest(*API+"/user", us)
					log.Printf("%s - %s", *API+"/user", r)
					if err != nil {
						log.Printf("ERROR: get %s/whitelist/", API, err)
					}
					fields = append(fields, slack.AttachmentField{
						Title: "",
						Value: fmt.Sprintf("The user <@%s> has been added", us.ID),
					})
					attachment := &slack.Attachment{
						Pretext: "",
						Color:   "#0a84c1",
						Fields:  fields,
					}
					attachmentChannel.Attachment = attachment
					c <- attachmentChannel
				}
			}
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
					Value: "Only admin can execute delete command",
				})
				attachment := &slack.Attachment{
					Pretext: "Current",
					Color:   "#0a84c1",
					Fields:  fields,
				}
				attachmentChannel.Attachment = attachment
				c <- attachmentChannel
			} else {
				if len(commandArray) < 3 {
					log.Printf("error: user is not specified")
					fields := make([]slack.AttachmentField, 0)
					fields = append(fields, slack.AttachmentField{
						Title: "",
						Value: "You must specify user: <bot> del <slack user name>",
					})
					attachment := &slack.Attachment{
						Pretext: "Error",
						Color:   "#0a84c1",
						Fields:  fields,
					}
					attachmentChannel.Attachment = attachment
					c <- attachmentChannel
				} else {

					slackUser, err := api.GetUserInfo(commandArray[2][2 : len(commandArray[2])-1])
					if err != nil {
						log.Printf("%s\n", err)
					}
					req, err := http.NewRequest("DELETE", *API+"/user/"+slackUser.ID, nil)
					if err != nil {
						log.Printf("%s\n", err)
					}
					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						log.Printf("%s\n", err)
					}
					log.Printf("%s", resp)
				}
			}
		case "admins":
			log.Println("admin")
			resp, err := http.Get(*API + "/users/admins")
			defer resp.Body.Close()
			if err != nil {
				log.Printf("ERROR: get %s/admins", API, err)
			}
			var active_users []UserInfo
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Printf("ERROR: can't read responce: %s", err)
			}
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
		default:
			fields := make([]slack.AttachmentField, 0)
			fields = append(fields, slack.AttachmentField{
				Title: "",
				Value: fmt.Sprintf("Not sure what do you mean: %s", commandArray[1]),
			})
			attachment := &slack.Attachment{
				Pretext: "Error",
				Color:   "#0a84c1",
				Fields:  fields,
			}
			attachmentChannel.Attachment = attachment
			c <- attachmentChannel
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
		_, _, errPostMessage := api.PostMessage(*channelId, ac.DisplayTitle, params)
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

	api = slack.New(*token)
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
		time.Sleep(time.Second * 1)
		select {
		case msg := <-rtm.IncomingEvents:
			switch ev := msg.Data.(type) {
			case *slack.ConnectedEvent:
				botId = ev.Info.User.ID
				//groups, _ := api.GetGroups(false)
				ip, err := getIpAddress()
				if err != nil {
					fmt.Printf("ERROR: getting IP address error: %s", err)
				}
				_, _, channel, err := rtm.OpenIMChannel(*userId)
				rtm.PostMessage(channel, "```"+botRestarted+ip+"```", params)
				//whyt?????
			case *slack.MessageEvent:
				switch s := ev.Channel[0]; string(s) {
				case "D":
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
						botCommandChannel <- botCentral
					}
				case "C":
				}
			case *slack.RTMError:
				log.Fatal("Error: %s\n", ev.Error())
			case *slack.LatencyReport:
				log.Printf("Current latency: %v\n", ev.Value)
			case *slack.InvalidAuthEvent:
				log.Fatal("Invalid credentials")
				break Loop

			default:
				// Ignore other events..
				// fmt.Printf("Unexpected: %v\n", msg.Data)
			}
		}
	}
}

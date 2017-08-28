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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/microservices/slack-bot-go/tickets"

	"github.com/nlopes/slack"
)

type User struct {
	Info slack.User
	//Rating int
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
type Field struct {
	Num   string `json:"num"`
	State string `json:"state"`
	Owner string `json:"owner"`
}
type jobs struct {
	//ID      string  `json:"_id"`
	//Owner   string  `json:"owner"`
	Tickets []Field `json:"tickets"`
}

/*
type Message struct {
	ChannelId string
	Timestamp string
	Payload   string
	Rating    int
	User      User
}
*/
type BotCentral struct {
	Group  *slack.Group
	Event  *slack.MessageEvent
	UserId string
}

type AttachmentChannel struct {
	Attachment   []slack.Attachment
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

const (
	dayOfWeek = 7
	ERROR     = "#ff5151"
	WARN      = "#f9ff03"
	OK        = "#36a64f"
)

func init() {
	userId = flag.String("user", "U03EPQS1F", "slack bot user id")
	channelId = flag.String("channel", "G2VLDKLSX", "slack channel ID")
	API = flag.String("api", "http://localhost:8080/api", "API url")
	token = flag.String("crednetial", "xoxb-88562823922-NYJLdNas6mwYuYiNjLVmPMWf", "slack bot token")

}
func parseActiveTickets(tickets []tickets.Ticket) ([]slack.AttachmentField, error) {
	url := "https://www.e-access.att.com/ushportal/search1.cfm?searchtype=SeeTkt&criteria"
	fields := make([]slack.AttachmentField, 0)
	var state map[string]int
	state = make(map[string]int)
	for _, ticket := range tickets {
		st := strings.Replace(strings.Split(ticket.State, "-")[0], " ", "", -1)
		state[st] = state[st] + 1
		opened := ticket.ISOOpened.Format(time.RFC3339)
		lastModified := ticket.ISOLastModified.Format(time.RFC3339)
		field := slack.AttachmentField{
			//Title: fmt.Sprintf("<%s=%s|#%s> - sev: %s - %s - %s", url, ticket.Number, ticket.Number, ticket.Sev, ticket.State, ticket.Owner),
			Value: fmt.Sprintf("```<%s=%s|#%s> - sev: %s - %s - %s\nOpened:  %s\t Last modified: %s```", url, ticket.Number, ticket.Number, ticket.Sev, ticket.State, ticket.Owner, opened, lastModified),
			Short: false,
		}
		fields = append(fields, field)
	}
	field := slack.AttachmentField{
		Title: fmt.Sprintf("Total: %d", len(tickets)),
		Value: fmt.Sprintf("Active : %d\t Deferred : %d\t Closed : %d\t Ready to Close: %d", state["Active"], state["Deferred"], state["Closed"], state["ReadytoClose"]),
		//Short: false,
	}
	fields = append(fields, field)
	return fields, nil
}
func parseTickets(tickets []tickets.Ticket, opened bool) ([]slack.AttachmentField, error) {
	url := "https://www.e-access.att.com/ushportal/search1.cfm?searchtype=SeeTkt&criteria"
	fields := make([]slack.AttachmentField, 0)
	var (
		ticketReport map[time.Weekday][]string
		state        map[string]int
		parseField   time.Weekday
		Weekdays     = []time.Weekday{
			time.Monday,
			time.Tuesday,
			time.Wednesday,
			time.Thursday,
			time.Friday,
			time.Saturday,
			time.Sunday,
		}
	)
	ticketReport = make(map[time.Weekday][]string, dayOfWeek)
	state = make(map[string]int)
	for _, ticket := range tickets {
		if opened {
			parseField = ticket.ISOOpened.Weekday()
			ticketReport[parseField] = append(ticketReport[parseField], fmt.Sprintf("<%s=%s|#%s> - %s - %s\n", url, ticket.Number, ticket.Number, ticket.State, ticket.Owner))
		} else {
			parseField = ticket.ISOClosed.Weekday()
			ticketReport[parseField] = append(ticketReport[parseField], fmt.Sprintf("<%s=%s|#%s> - %s\n", url, ticket.Number, ticket.Number, ticket.Owner))
		}
		st := strings.Replace(strings.Split(ticket.State, "-")[0], " ", "", -1)
		state[st] = state[st] + 1
	}
	for _, day := range Weekdays {
		if len(ticketReport[day]) != 0 {
			field := slack.AttachmentField{
				Title: fmt.Sprintf("%s -> %d", day.String(), len(ticketReport[day])),
				Value: fmt.Sprintf("```\n%s```", strings.Join(ticketReport[day], "")),
				Short: false,
			}
			fields = append(fields, field)
		}
	}
	if opened {
		field := slack.AttachmentField{
			Title: fmt.Sprintf("Total: %d", len(tickets)),
			Value: fmt.Sprintf("Active : %d\t Deferred : %d\t Closed : %d\t Ready to Close: %d", state["Active"], state["Deferred"], state["Closed"], state["Ready to Close"]),
			//Short: false,
		}
		fields = append(fields, field)
	} else {
		field := slack.AttachmentField{
			Title: fmt.Sprintf("Total: %d", len(tickets)),
			//Short: false,
		}
		fields = append(fields, field)
	}
	return fields, nil
}
func parseWorkload(workload []jobs) ([]slack.AttachmentField, error) {
	url := "https://www.e-access.att.com/ushportal/search1.cfm?searchtype=SeeTkt&criteria"
	fields := make([]slack.AttachmentField, 0)
	for _, job := range workload {
		owner := fmt.Sprintf("%s: %d", job.Tickets[0].Owner, len(job.Tickets))
		workload := "```"
		for _, ticket := range job.Tickets {
			workload = fmt.Sprintf("%s\n<%s=%s|#%s> - %s", workload, url, ticket.Num, ticket.Num, ticket.State)
		}
		field := slack.AttachmentField{
			Title: owner,
			Value: fmt.Sprintf("%s```", workload),
			Short: false,
		}
		fields = append(fields, field)
	}
	return fields, nil

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
func permissionDenidedMessage() slack.Attachment {
	fields := make([]slack.AttachmentField, 0)
	fields = append(fields, slack.AttachmentField{
		Title: "",
		Value: "",
	})
	attachment := buildMessage("Only admin can execute the command", "#0a84c1", fields)
	return attachment
}
func buildMessage(prefix string,
	color string,
	fields []slack.AttachmentField) slack.Attachment {
	attachment := slack.Attachment{
		Pretext:    prefix,
		Color:      color,
		Fields:     fields,
		MarkdownIn: []string{"pretext", "text", "fields"},
	}
	return attachment
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
		resp, err := getRequest(*API + "/user/isadmin/" + botChannel.UserId)
		if err != nil {
			log.Printf("error: is admin check error: %s", err)
		}
		isadmin, err := strconv.ParseBool(string(resp))
		if err != nil {
			log.Printf("error: can't parse bool %s", err)
		}
		switch commandArray[1] {
		case "help": // works
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
			attachment := buildMessage("Command List", OK, fields)
			attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
			c <- attachmentChannel

		case "current": //works
			fields := make([]slack.AttachmentField, 0)
			resp, err := getRequest(*API + "/users/current")
			if err != nil {
				log.Printf("error: error during current command: %s", resp)
			}
			var user UserInfo

			err = json.Unmarshal(resp, &user)
			if err != nil {
				log.Printf("error: can't parse infromation about the user: %s", err)
			}
			fields = append(fields, slack.AttachmentField{
				Title: "",
				Value: "We are waiting for <@" + user.ID + "> to grab a ticket",
			})
			attachment := buildMessage("Current", "#0a84c1", fields)
			attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
			c <- attachmentChannel
		case "order": //works
			resp, err := getRequest(*API + "/users/active")
			if err != nil {
				log.Printf("error: error during order %s", err)
			}
			var active_users []UserInfo
			err = json.Unmarshal(resp, &active_users)
			if err != nil {
				log.Printf("error: can't parse infromation about the user: %s", err)
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
			attachment := buildMessage("Order of active members in the rotation.", "#0a84c1", fields)
			attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
			resp, err = getRequest(*API + "/users/blacklisted")
			if err != nil {
				log.Printf("error: %s/blacklisted - %s", *API, err)
			}
			var blacklisted_users []UserInfo
			err = json.Unmarshal(resp, &blacklisted_users)
			if err != nil {
				log.Printf("error: can't parse infromation about the user: %s", err)
			}
			number_of_users = len(blacklisted_users)

			fields = make([]slack.AttachmentField, number_of_users)
			for i := 0; i < number_of_users; i++ {
				field := slack.AttachmentField{
					Title: "",
					Value: fmt.Sprintf("%s", blacklisted_users[i].RealName),
					//Short: false,
				}
				fields[i] = field
			}
			attachment = buildMessage("Blacklisted members", "#0a84c1", fields)
			attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
			c <- attachmentChannel
		case "next": // works
			if !isadmin {
				attachment := permissionDenidedMessage()
				attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
				c <- attachmentChannel
			} else {
				fields := make([]slack.AttachmentField, 0)
				resp, err := getRequest(*API + "/users/next")
				if err != nil {
					log.Printf("error: get %s/current - %s", *API, err)
				}
				var user UserInfo
				err = json.Unmarshal(resp, &user)
				if err != nil {
					log.Printf("error: can't parse infromation about the user: %s", err)
				}
				fields = append(fields, slack.AttachmentField{
					Title: "",
					Value: "We are waiting for <@" + user.ID + "> to grab a ticket",
				})
				attachment := buildMessage("Next", "#0a84c1", fields)
				attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
				c <- attachmentChannel
			}
		case "blacklist": //works
			if !isadmin {
				attachment := permissionDenidedMessage()
				attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
				c <- attachmentChannel
			} else {
				if len(commandArray) < 3 {
					log.Printf("error: user is not specified")
					fields := make([]slack.AttachmentField, 0)
					fields = append(fields, slack.AttachmentField{
						Title: "",
						Value: "You must specify user",
					})
					attachment := buildMessage("Error", "#0a84c1", fields)
					attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
					c <- attachmentChannel
				} else {
					// get information about the user. Need to have slack ID and user name to print this information.
					// possibly need to change api service.

					// ^ implementation should be there.
					mentionedUser := commandArray[2][2 : len(commandArray[2])-1]
					printedUserName, err := api.GetUserInfo(mentionedUser)
					if err != nil {
						log.Printf("error: can't get information about the user %s", err)
						return
					}
					fields := make([]slack.AttachmentField, 0)
					_, err = getRequest(*API + "/user/blacklist/" + mentionedUser)
					if err != nil {
						log.Printf("error: get %s/blacklist/ - %s", *API, err)
					}
					fields = append(fields, slack.AttachmentField{
						Title: "",
						Value: fmt.Sprintf("The user %s has been blacklisted", printedUserName.Profile.RealName),
					})
					attachment := buildMessage("", "#0a84c1", fields)
					attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
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
				attachment := permissionDenidedMessage()
				attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
				c <- attachmentChannel
			} else {
				if len(commandArray) < 3 {
					log.Printf("error: user is not specified")
					fields := make([]slack.AttachmentField, 0)
					fields = append(fields, slack.AttachmentField{
						Title: "",
						Value: "You must specify user",
					})
					attachment := buildMessage("Error", "#0a84c1", fields)
					attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
					c <- attachmentChannel
				} else {
					mentionedUser := commandArray[2][2 : len(commandArray[2])-1]
					fields := make([]slack.AttachmentField, 0)
					_, err := getRequest(*API + "/user/whitelist/" + mentionedUser)
					if err != nil {
						log.Printf("error: get %s/whitelist/ - %s", *API, err)
					}
					fields = append(fields, slack.AttachmentField{
						Title: "",
						Value: fmt.Sprintf("The user <@%s> has been whitelisted", mentionedUser),
					})
					attachment := buildMessage("", "#0a84c1", fields)
					attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
					c <- attachmentChannel
				}
			}
		case "add":
			if !isadmin {
				attachment := permissionDenidedMessage()
				attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
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
					attachment := buildMessage("Error", "#0a84c1", fields)
					attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
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
					_, err = postRequest(*API+"/user", us)
					if err != nil {
						log.Printf("error: get %s/whitelist/ - %s", *API, err)
					}
					fields = append(fields, slack.AttachmentField{
						Title: "",
						Value: fmt.Sprintf("The user <@%s> has been added", us.ID),
					})
					attachment := buildMessage("", "#0a84c1", fields)
					attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
					c <- attachmentChannel
				}
			}
			if !isadmin {
				attachment := permissionDenidedMessage()
				attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
				c <- attachmentChannel
			} else {
			}
		case "del":
			if !isadmin {
				attachment := permissionDenidedMessage()
				attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
				c <- attachmentChannel
			} else {
				if len(commandArray) < 3 {
					log.Printf("error: user is not specified")
					fields := make([]slack.AttachmentField, 0)
					fields = append(fields, slack.AttachmentField{
						Title: "",
						Value: "You must specify user: <bot> del <slack user name>",
					})
					attachment := buildMessage("Error", "#0a84c1", fields)
					attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
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
					_, err = http.DefaultClient.Do(req)
					if err != nil {
						log.Printf("%s\n", err)
					}
				}
			}
		case "admins":
			resp, err := http.Get(*API + "/users/admins")
			defer resp.Body.Close()
			if err != nil {
				log.Printf("error: get %s/admins - %s", *API, err)
			}
			var active_users []UserInfo
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Printf("error: can't read responce: %s", err)
			}
			err = json.Unmarshal(body, &active_users)
			if err != nil {
				log.Printf("error: can't parse infromation about the user: %s", err)
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
			attachment := buildMessage("Admins", "#0a84c1", fields)
			attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
			c <- attachmentChannel
		case "tickets":
			year, week := time.Now().ISOWeek()
			r, err := getRequest(fmt.Sprintf("%s/tickets/report/%d/%d", *API, year, week))
			if err != nil {
				log.Printf("%s", err)
			}
			var tickets []tickets.Ticket
			err = json.Unmarshal(r, &tickets)
			if err != nil {
				log.Printf("%s", err)
			}
			fields, err := parseTickets(tickets, true)
			if err != nil {
				log.Printf("%s", err)
			}
			attachment := buildMessage("Opened tickets", "#0a84c1", fields)
			attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
			r, err = getRequest(fmt.Sprintf("%s/tickets/reportclosed/%d/%d", *API, year, week))
			if err != nil {
				log.Printf("%s", err)
			}
			err = json.Unmarshal(r, &tickets)
			if err != nil {
				log.Printf("%s", err)
			}
			fields, err = parseTickets(tickets, false)
			if err != nil {
				log.Printf("%s", err)
			}
			attachment = buildMessage("Closed tickets", "#0a84c1", fields)
			attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
			c <- attachmentChannel
		case "last":
			year, week := time.Now().ISOWeek()
			week = week - 1
			if week < 0 {
				week = 53
				year = year - 1
			}
			r, err := getRequest(fmt.Sprintf("%s/tickets/report/%d/%d", *API, year, week))
			if err != nil {
				log.Printf("%s", err)
			}
			var tickets []tickets.Ticket
			err = json.Unmarshal(r, &tickets)
			if err != nil {
				log.Printf("%s", err)
			}
			fields, err := parseTickets(tickets, true)
			if err != nil {
				log.Printf("%s", err)
			}
			attachment := buildMessage("Openned tickets", "#0a84c1", fields)
			attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
			r, err = getRequest(fmt.Sprintf("%s/tickets/reportclosed/%d/%d", *API, year, week))
			if err != nil {
				log.Printf("%s", err)
			}
			err = json.Unmarshal(r, &tickets)
			if err != nil {
				log.Printf("%s", err)
			}
			fields, err = parseTickets(tickets, false)
			if err != nil {
				log.Printf("%s", err)
			}
			attachment = buildMessage("Closed tickets", "#0a84c1", fields)
			attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
			c <- attachmentChannel
		case "backlog":
			r, err := getRequest(fmt.Sprintf("%s/tickets/active", *API))
			if err != nil {
				log.Printf("%s", err)
			}
			var tickets []tickets.Ticket
			err = json.Unmarshal(r, &tickets)
			if err != nil {
				log.Printf("%s", err)
			}
			fields, err := parseActiveTickets(tickets)
			if err != nil {
				log.Printf("%s", err)
			}
			attachment := buildMessage("Backlog", OK, fields)
			attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
			c <- attachmentChannel
		case "count":
			r, err := getRequest(fmt.Sprintf("%s/workload", *API))
			if err != nil {
				log.Printf("%s", err)
			}
			var workload []jobs
			err = json.Unmarshal(r, &workload)
			if err != nil {
				log.Printf("%s", err)
			}
			fields, err := parseWorkload(workload)
			if err != nil {
				log.Printf("%s", err)
			}
			attachment := buildMessage("Count", OK, fields)
			attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
			c <- attachmentChannel
		default:
			fields := make([]slack.AttachmentField, 0)
			fields = append(fields, slack.AttachmentField{
				Title: "",
				Value: fmt.Sprintf("Not sure what do you mean: %s", commandArray[1]),
			})
			attachment := buildMessage("Error", ERROR, fields)
			attachmentChannel.Attachment = append(attachmentChannel.Attachment, attachment)
			c <- attachmentChannel
		}
		attachmentChannel.Attachment = nil
	}
}

func handleBotReply() {
	for {
		ac := <-botReplyChannel
		params := slack.PostMessageParameters{}
		params.Markdown = true
		params.AsUser = true
		params.Attachments = ac.Attachment
		_, _, errPostMessage := api.PostMessage(*channelId, ac.DisplayTitle, params)
		if errPostMessage != nil {
			log.Printf("error: error during post message %s", errPostMessage)
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
	flag.Parse()
	logger := log.New(os.Stdout, "slack-bot: ", log.Lshortfile|log.LstdFlags)
	slack.SetLogger(logger)

	api = slack.New(*token)
	api.SetDebug(true)

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
				//groups, _ := api.GetGroups(false)
				ip, err := getIpAddress()
				if err != nil {
					fmt.Printf("error: getting IP address error: %s", err)
				}
				_, _, channel, err := rtm.OpenIMChannel(*userId)
				rtm.PostMessage(channel, "```"+botRestarted+ip+"```", params)
				//whyt?????
			case *slack.MessageEvent:
				switch s := ev.Channel[0]; string(s) {
				case "D":
				case "G":
					groupInfo, err := api.GetGroupInfo(ev.Channel)
					if err != nil {
						log.Printf("error: getting message error %s", err)
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
				log.Fatal(ev.Error())
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

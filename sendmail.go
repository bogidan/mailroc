package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	// Email
	"net/smtp"
	"github.com/jordan-wright/email"
	// Configuration
	"os"
	"syscall"
	"time"
	"github.com/BurntSushi/toml"
	"github.com/boltdb/bolt"
	"golang.org/x/crypto/ssh/terminal"
)

type Config struct {
	Name     string
	Email    string
	Imap     string `toml:"imap_server"`
	Smtp     string `toml:"smtp_server"`
	Username string
	Password string
	Echo     bool
}

type Options struct {
	Subject    *string
	Body       *string
	To         *string
	Cc         *string
	Bcc        *string
	ConfigFile *string
}

func readConfig(configfile *string) Config {
	_, err := os.Stat(*configfile)
	if err != nil {
		log.Fatal("Config file is missing: ", *configfile)
	}

	var config Config
	if _, err := toml.DecodeFile(*configfile, &config); err != nil {
		log.Fatal(err)
	}

	return config
}

func readOptions() (Options, []string) {
	var o Options
	o.Subject = flag.String("s", "Code Review", "Email Subject")
	o.Body = flag.String("b", "", "Email Body")
	o.To = flag.String("to", "bogdanfilipchuk@gmail.com", "Comma separated emails of recipients")
	o.Cc = flag.String("cc", "", "Comma separated emails of Cc")
	o.Bcc = flag.String("bcc", "", "Comma separated emails for Bcc")
	o.ConfigFile = flag.String("config", "sendmail.toml", "Toml file with account configuration")
	flag.Parse()
	return o, flag.Args()
}

func splitEmails(list string) []string {
	if list == "" {
		return []string{}
	}
	return strings.Split(list, ",")
}


func AddAccount(db *bolt.DB, email string) (pass string) {
	db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("Accounts"))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}

		var key = []byte(email)
		val := b.Get(key)
		if val != nil {
			fmt.Printf("got value: %s", val)
			return nil
		}

		//reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter password for '"+ email +"': ")
		bytepass, err := terminal.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("enter password: %s", err)
		}
		pass = string(bytepass)

		err = b.Put(key, bytepass)
		if err != nil {
			return fmt.Errorf("put key: %s", err)
		}
		return nil
	})
	return
}

func main() {
	var err error
	options, args := readOptions()
	config := readConfig(options.ConfigFile)
	db, err := bolt.Open("mail.db", 0600, &bolt.Options{Timeout: 5 * time.Microsecond})
	if err == bolt.ErrTimeout { // Other instance open.
		log.Fatal("Another instance is open")
	} else if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Get/Create Accounts Bucket
	var pass = AddAccount(db, config.Email)

	fmt.Printf("got value: %s", pass)
	return

	// Authenticate Connection
	auth := LoginAuth(config.Username, config.Password)

	e := email.NewEmail()
	e.From = config.Email
	e.To = splitEmails(*options.To)
	e.Cc = splitEmails(*options.Cc)
	e.Bcc = splitEmails(*options.Bcc)
	if config.Echo {
		e.Bcc = append(e.Bcc, config.Email)
	}
	e.Subject = *options.Subject
	e.Text = []byte(*options.Body)
	e.HTML = []byte("")

	for _, arg := range args {
		_, err = e.AttachFile(arg)
		if err != nil {
			log.Fatal(err)
		}
	}

	//e.Send("smtp.gmail.com:587", smtp.PlainAuth("", "test@gmail.com", "password123", "smtp.gmail.com"))
	err = e.Send(config.Smtp, auth)
	if err != nil {
		log.Fatal(err)
	}
}

type loginAuth struct {
	username, password string
}

// loginAuth returns an Auth that implements the LOGIN authentication
// mechanism as defined in RFC 4616.
func LoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username, password}
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	command := string(fromServer)
	command = strings.TrimSpace(command)
	command = strings.TrimSuffix(command, ":")
	command = strings.ToLower(command)

	if more {
		if command == "username" {
			return []byte(fmt.Sprintf("%s", a.username)), nil
		} else if command == "password" {
			return []byte(fmt.Sprintf("%s", a.password)), nil
		} else {
			// We've already sent everything.
			return nil, fmt.Errorf("unexpected server challenge: %s", command)
		}
	}
	return nil, nil
}

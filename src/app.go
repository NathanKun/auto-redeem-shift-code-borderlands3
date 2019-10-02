package main

import (
	"catprogrammer.com/auto-redeem-shift-code-borderlands3/credentials"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"github.com/mmcdole/gofeed"
	"io/ioutil"
	"log"
	"net"
	"net/smtp"
	"os"
	"time"
)

const (
	filename         string = "lastcode.txt"
	rssUrl           string = "https://shift.orcicorn.com/tags/borderlands3/index.xml"
	emailSubject     string = "Auto Redeem Shift Code Borderlands3"
	emailContentType string = "text/html; charset=UTF-8"
	emailBody        string = `
			<html><body>
			<h2>Possible SHiFT code</h2>
			<h3>%s</h3>
			<h2>Description</h2>
			<p>%s</p>
			</body></html>
			`
	chromedpTimeoutSec time.Duration = 30
)

func main() {
	log.Println("Start")
	feedItem := readFeed()
	latestcode := feedItem.Title
	savedcode := readLastUsedShiftCode()

	if len(savedcode) == 0 || savedcode != latestcode {
		log.Println("Found new SHiFT code ", latestcode)
		log.Println("Redeem code")
		redeemCode(latestcode)
		log.Println("Send Email")
		sendEmail(feedItem)
		writeUsedShiftCode(latestcode)
	}

	log.Println("End")
}

// read saved SHiFT code file
func readLastUsedShiftCode() string {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		err = ioutil.WriteFile(filename, []byte(""), 0755)
		if err != nil {
			log.Panic("Can not create file ", filename, err)
		} else {
			log.Println("Created file ", filename)
		}
	}

	b, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Panic("Can not read file ", filename, err)
	}

	data := string(b)
	log.Println("Read SHiFT code: ", data)
	return data
}

// read SHiFT code RSS feed
func readFeed() *gofeed.Item {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(rssUrl)

	if err != nil {
		log.Panic("Can not parse RSS feed url ", rssUrl, err)
	}

	if len(feed.Items) > 0 {
		return feed.Items[0]
	} else {
		log.Panic("Feed items length is 0. Feed = ", feed)
	}

	return nil
}

func redeemCode(code string) {
	// create context
	ctx, cancel := chromedp.NewContext(context.Background(), chromedp.WithLogf(log.Printf))
	defer cancel()

	// create a timeout
	ctx, cancel = context.WithTimeout(ctx, chromedpTimeoutSec*time.Second)
	defer cancel()

	// run task list
	var res string
	var msgNodes []*cdp.Node
	err := chromedp.Run(ctx, chromedp.Tasks{
		// login page
		chromedp.Navigate("https://shift.gearboxsoftware.com/home"),
		chromedp.WaitVisible("body > footer"),
		chromedp.SendKeys("#user_email", credentials.GearboxLogin),
		chromedp.SendKeys("#user_password", credentials.GearboxPassword),
		chromedp.Click(`//input[@name="commit"]`),
		chromedp.WaitVisible("#current_shift_service_id"),
		chromedp.Text("#current_shift_service_id", &res),
		// reward page
		chromedp.Navigate("https://shift.gearboxsoftware.com/rewards"),
		chromedp.WaitVisible("#shift_code_check"),
		// find & click redeem button
		chromedp.SendKeys("#shift_code_input", code),
		chromedp.Click("#shift_code_check"),
		chromedp.Sleep(time.Second * 2),
		chromedp.Nodes("#code_results", &msgNodes),
		chromedp.Click(`//input[@name="commit"]`),
		chromedp.Sleep(time.Second * 10),
	})

	// TODO: check if redeem success

	if err != nil {
		if err.Error() == "context deadline exceeded" {
			log.Println("context deadline exceeded")
			if len(msgNodes) > 0 && len(msgNodes[0].Children) > 0 {
				log.Println(msgNodes[0].Children[0].NodeValue)
			}
			log.Println("msgNodes: ")
			log.Println(msgNodes)
		} else {
			log.Panic(err)
		}
	}
}

func sendEmail(feedItem *gofeed.Item) {
	headers := make(map[string]string)
	headers["From"] = credentials.SmtpUser
	headers["To"] = credentials.SmtpSendTo
	headers["Subject"] = emailSubject
	headers["Content-Type"] = emailContentType

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += fmt.Sprintf(emailBody, feedItem.Title, feedItem.Description)

	log.Println("message:\n" + message)

	host, _, _ := net.SplitHostPort(credentials.SmtpServer)
	auth := smtp.PlainAuth("", credentials.SmtpUser, credentials.SmtpPassword, host)

	log.Println("host:", host)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}

	// ssl connection
	conn, err := tls.Dial("tcp", credentials.SmtpServer, tlsConfig)
	if err != nil {
		log.Panic("Send email error.", err)
	}

	// smtp client that use the ssl connection
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		log.Panic("Send email error.", err)
	}

	// auth
	if err := c.Auth(auth); err != nil {
		log.Panic("Send email error.", err)
	}

	// from, to
	if err = c.Mail(credentials.SmtpUser); err != nil {
		log.Panic("Send email error.", err)
	}

	if err = c.Rcpt(credentials.SmtpSendTo); err != nil {
		log.Panic("Send email error.", err)
	}

	// content
	w, err := c.Data()
	if err != nil {
		log.Panic("Send email error.", err)
	}

	_, err = w.Write([]byte(message))
	if err != nil {
		log.Panic("Send email error.", err)
	}

	err = w.Close()
	if err != nil {
		log.Panic("Send email error.", err)
	}

	err = c.Quit()
	if err != nil {
		log.Panic("Send email error.", err)
	}
}

func writeUsedShiftCode(code string) {
	err := ioutil.WriteFile(filename, []byte(code), 0755)
	if err != nil {
		log.Panic("Can not write to file ", filename, err)
	} else {
		log.Println("Wrote latest SHiFT code to file ", filename)
	}
}

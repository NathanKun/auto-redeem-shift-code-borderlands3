package main

import (
	"catprogrammer.com/auto-redeem-shift-code-borderlands3/credentials"
	"context"
	"crypto/tls"
	"fmt"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"github.com/mmcdole/gofeed"
	"github.com/op/go-logging"
	"io/ioutil"
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
			<h2>Redeem Result</h2>
			<p>%s</p>
			<h2>Redeem Notice</h2>
			<p>%s</p>
			</body></html>
			`
	chromedpTimeoutSec time.Duration = 30
)

var log = logging.MustGetLogger("example")

func main() {
	logBackend := logging.AddModuleLevel(logging.NewLogBackend(os.Stderr, "", 0))
	if (credentials.LogLevel == "INFO") {
		logBackend.SetLevel(logging.INFO, "")
	} else {
		logBackend.SetLevel(logging.ERROR, "")
	}
	logging.SetBackend(logBackend)

	log.Info("Start")
	feedItem := readFeed()
	latestcode := feedItem.Extensions["archive"]["shift"][0].Children["code"][0].Value
	savedcode := readLastUsedShiftCode()

	if len(savedcode) == 0 || savedcode != latestcode {
		log.Info("Found new SHiFT code", latestcode)
		log.Info("Redeem code")
		redeemResult, redeemNotice := redeemCode(latestcode)
		log.Info("Send Email")
		sendEmail(feedItem, redeemResult, redeemNotice)
		writeUsedShiftCode(latestcode)
	} else {
		log.Info("No new SHiFT code")
	}

	log.Info("End")
}

// read saved SHiFT code file
func readLastUsedShiftCode() string {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		err = ioutil.WriteFile(filename, []byte(""), 0755)
		if err != nil {
			log.Error("Can not create file ", filename, err)
			panic(err)
		} else {
			log.Info("Created file ", filename)
		}
	}

	b, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Error("Can not read file ", filename, err)
		panic(err)
	}

	data := string(b)
	log.Info("Read SHiFT code: ", data)
	return data
}

// read SHiFT code RSS feed
func readFeed() *gofeed.Item {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(rssUrl)

	if err != nil {
		log.Error("Can not parse RSS feed url ", rssUrl, err)
		panic(err)
	}

	if len(feed.Items) > 0 {
		return feed.Items[0]
	} else {
		log.Error("Feed items length is 0. Feed = ", feed)
		panic(err)
	}

	return nil
}

func redeemCode(code string) (string, string) {
	// create context
	ctx, cancel := chromedp.NewContext(context.Background(), chromedp.WithLogf(log.Infof))
	defer cancel()

	// create a timeout
	ctx, cancel = context.WithTimeout(ctx, chromedpTimeoutSec*time.Second)
	defer cancel()

	// run task list
	var res string
	var msgNodes []*cdp.Node
	var noticeNodes []*cdp.Node
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
		chromedp.Nodes(".alert.notice", &noticeNodes),
	})

	// TODO: check if redeem success
	notice := ""
	if (len(noticeNodes) > 0) {
		notice = noticeNodes[0].NodeValue
	}

	if err != nil {
		if err.Error() == "context deadline exceeded" {
			log.Info("context deadline exceeded")
			if len(msgNodes) > 0 && len(msgNodes[0].Children) > 0 {
				log.Info(msgNodes[0].Children[0].NodeValue)
			}
			return msgNodes[0].XMLVersion, notice
		} else {
			log.Info(err)
			return err.Error(), notice
		}
	}

	return "OK", notice
}

func sendEmail(feedItem *gofeed.Item, redeemResult string, redeemNotice string) {
	headers := make(map[string]string)
	headers["From"] = credentials.SmtpUser
	headers["To"] = credentials.SmtpSendTo
	headers["Subject"] = emailSubject
	headers["Content-Type"] = emailContentType

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += fmt.Sprintf(emailBody, feedItem.Title, feedItem.Description, redeemResult, redeemNotice)

	host, _, _ := net.SplitHostPort(credentials.SmtpServer)
	auth := smtp.PlainAuth("", credentials.SmtpUser, credentials.SmtpPassword, host)

	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         host,
	}

	// ssl connection
	conn, err := tls.Dial("tcp", credentials.SmtpServer, tlsConfig)
	if err != nil {
		log.Error("Send email error.", err)
		panic(err)
	}

	// smtp client that use the ssl connection
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		log.Error("Send email error.", err)
		panic(err)
	}

	// auth
	if err := c.Auth(auth); err != nil {
		log.Error("Send email error.", err)
		panic(err)
	}

	// from, to
	if err = c.Mail(credentials.SmtpUser); err != nil {
		log.Error("Send email error.", err)
		panic(err)
	}

	if err = c.Rcpt(credentials.SmtpSendTo); err != nil {
		log.Error("Send email error.", err)
		panic(err)
	}

	// content
	w, err := c.Data()
	if err != nil {
		log.Error("Send email error.", err)
		panic(err)
	}

	_, err = w.Write([]byte(message))
	if err != nil {
		log.Error("Send email error.", err)
		panic(err)
	}

	err = w.Close()
	if err != nil {
		log.Error("Send email error.", err)
		panic(err)
	}

	err = c.Quit()
	if err != nil {
		log.Error("Send email error.", err)
		panic(err)
	}
}

func writeUsedShiftCode(code string) {
	err := ioutil.WriteFile(filename, []byte(code), 0755)
	if err != nil {
		log.Error("Can not write to file ", filename, err)
		panic(err)
	} else {
		log.Info("Wrote latest SHiFT code to file ", filename)
	}
}

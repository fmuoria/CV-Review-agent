// Package posting writes job-ad copy for each channel, with a tracked apply
// link per channel so the dashboard can show where applicants came from.
//
// LinkedIn and BrighterMonday do not let third-party apps post on Sinapis's
// behalf, so HR copies this text into each site (about two minutes per site).
// The careers page on this app is updated automatically.
package posting

import (
	"fmt"
	"strings"

	"github.com/fmuoria/CV-Review-agent/internal/store"
)

// Channel is a place to post a job.
type Channel struct {
	Key, Label, Instructions, PostURL string
}

// Channels lists the supported posting channels.
var Channels = []Channel{
	{Key: "linkedin", Label: "LinkedIn", PostURL: "https://www.linkedin.com/talent/post-a-job",
		Instructions: "Open LinkedIn → Post a free job (or your Recruiter/Jobs account). Paste the title and description. Under \"How would you like to receive applicants?\" choose \"External website\" and paste the apply link."},
	{Key: "brightermonday", Label: "BrighterMonday", PostURL: "https://www.brightermonday.co.ke/employer",
		Instructions: "Sign in to the BrighterMonday employer account → Post a job. Paste the description. Where it asks how candidates should apply, choose the external link / redirect option and paste the apply link. CVs that still arrive inside BrighterMonday can be downloaded and added with Bulk upload (source: BrighterMonday)."},
	{Key: "website", Label: "Sinapis website", PostURL: "",
		Instructions: "This job is listed automatically on the careers page while it is open. To show it on sinapis.org, link to the careers page or this job's page from the website's Careers menu."},
	{Key: "email", Label: "Email / WhatsApp", PostURL: "",
		Instructions: "Share this text with networks, partners and alumni groups. The link tracks applicants as referrals."},
}

// ApplyURL returns the tracked apply link for a channel.
func ApplyURL(baseURL string, job store.Job, channel string) string {
	src := channel
	if channel == "email" {
		src = "referral"
	}
	return fmt.Sprintf("%s/careers/%s?src=%s", strings.TrimRight(baseURL, "/"), job.Slug, src)
}

// Copy is the generated text for one channel.
type Copy struct {
	Channel  Channel
	Title    string
	Body     string
	ApplyURL string
}

// Generate returns copy for every channel.
func Generate(baseURL, orgName string, job store.Job) []Copy {
	out := make([]Copy, 0, len(Channels))
	for _, ch := range Channels {
		url := ApplyURL(baseURL, job, ch.Key)
		out = append(out, Copy{Channel: ch, Title: titleLine(job), Body: body(orgName, job, url, ch.Key), ApplyURL: url})
	}
	return out
}

func titleLine(j store.Job) string {
	if j.Location != "" {
		return fmt.Sprintf("%s — %s", j.Title, j.Location)
	}
	return j.Title
}

func body(org string, j store.Job, url, channel string) string {
	var b strings.Builder
	if channel == "email" {
		fmt.Fprintf(&b, "%s is hiring: %s\n\n", org, titleLine(j))
	}
	if j.Summary != "" {
		b.WriteString(j.Summary + "\n\n")
	}
	if j.Description != "" {
		b.WriteString(j.Description + "\n\n")
	}
	section(&b, "What you will do", j.RequiredDuties)
	section(&b, "What you will bring", append(append([]string{}, j.RequiredExperience...), j.RequiredEducation...))
	section(&b, "It would be great if you also have", append(append(append([]string{}, j.NiceExperience...), j.NiceEducation...), j.NiceDuties...))
	meta := []string{}
	if j.Location != "" {
		meta = append(meta, "Location: "+j.Location)
	}
	if j.EmploymentType != "" {
		meta = append(meta, "Type: "+j.EmploymentType)
	}
	if j.ClosesOn != nil {
		meta = append(meta, "Closing date: "+j.ClosesOn.Format("2 January 2006"))
	}
	if len(meta) > 0 {
		b.WriteString(strings.Join(meta, "\n") + "\n\n")
	}
	fmt.Fprintf(&b, "How to apply: submit your CV and cover letter at %s\n", url)
	if channel != "website" {
		b.WriteString("Applications sent by other means may not be considered.\n")
	}
	return strings.TrimSpace(b.String())
}

func section(b *strings.Builder, heading string, items []string) {
	if len(items) == 0 {
		return
	}
	b.WriteString(heading + ":\n")
	for _, it := range items {
		b.WriteString("• " + strings.TrimSpace(it) + "\n")
	}
	b.WriteString("\n")
}

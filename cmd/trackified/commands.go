package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/trackified/trackified-go"
)

var commands map[string]func(*cli, []string) error

func init() {
	commands = map[string]func(*cli, []string) error{
		"login":          cmdLogin,
		"track":          cmdTrack,
		"get":            cmdGet,
		"list":           cmdList,
		"watch":          cmdWatch,
		"retrack":        cmdRetrack,
		"stop":           cmdStop,
		"carriers":       cmdCarriers,
		"detect":         cmdDetect,
		"webhooks":       cmdWebhooks,
		"usage":          cmdUsage,
		"verify-webhook": cmdVerifyWebhook,
		"version":        cmdVersion,
	}
}

func table(w io.Writer, header string, rows func(tw io.Writer)) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, header)
	rows(tw)
	tw.Flush()
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func printTracking(w io.Writer, t *trackified.Tracking, events bool) {
	fmt.Fprintf(w, "%s  %s  %s  %s\n", t.ID, t.TrackingNumber, orDash(t.Carrier), t.Status)
	if t.EstimatedDelivery != "" {
		fmt.Fprintf(w, "Estimated delivery: %s\n", t.EstimatedDelivery)
	}
	if t.DeliveredAt != "" {
		fmt.Fprintf(w, "Delivered: %s\n", t.DeliveredAt)
	}
	if t.Exception != "" {
		fmt.Fprintf(w, "Exception: %s\n", t.Exception)
	}
	if events && len(t.Events) > 0 {
		fmt.Fprintln(w)
		for _, e := range t.Events {
			printEvent(w, e)
		}
	}
}

func printEvent(w io.Writer, e trackified.TrackingEvent) {
	fmt.Fprintf(w, "%-20s  %-18s  %s  %s\n", orDash(e.OccurredAt), e.Status, e.Description, e.Location)
}

func cmdLogin(c *cli, args []string) error {
	if _, err := parse(c.flags("login"), args, 0); err != nil {
		return err
	}
	key, err := readSecret(c.stdin, os.Stderr)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(key, "tk_") {
		return usageError("API keys start with tk_live_ or tk_test_")
	}
	client, err := trackified.New(trackified.WithAPIKey(key), trackified.WithUserAgent("trackified-cli/"+version))
	if err != nil {
		return err
	}
	u, err := client.Usage.Get(c.ctx)
	if err != nil {
		return fmt.Errorf("key rejected: %w", err)
	}
	p, err := saveAPIKey(key)
	if err != nil {
		return err
	}
	return c.emit(map[string]string{"config": p, "plan": u.Plan}, func() {
		fmt.Fprintf(c.out, "Logged in (%s plan). Key saved to %s\n", u.Plan, p)
	})
}

func cmdTrack(c *cli, args []string) error {
	fs := c.flags("track")
	carrier := fs.String("carrier", "", "carrier id (detected when empty)")
	customID := fs.String("custom-id", "", "your own reference")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	client, err := c.client()
	if err != nil {
		return err
	}
	t, err := client.Trackings.Create(c.ctx, &trackified.CreateTrackingParams{TrackingNumber: pos[0], Carrier: *carrier, CustomID: *customID})
	if err != nil {
		return err
	}
	return c.emit(t, func() { printTracking(c.out, t, true) })
}

func idCommand(name string, call func(*cli, *trackified.Client, string) (any, func(), error)) func(*cli, []string) error {
	return func(c *cli, args []string) error {
		pos, err := parse(c.flags(name), args, 1)
		if err != nil {
			return err
		}
		client, err := c.client()
		if err != nil {
			return err
		}
		v, human, err := call(c, client, pos[0])
		if err != nil {
			return err
		}
		return c.emit(v, human)
	}
}

var cmdGet = idCommand("get", func(c *cli, cl *trackified.Client, id string) (any, func(), error) {
	t, err := cl.Trackings.Get(c.ctx, id)
	return t, func() { printTracking(c.out, t, true) }, err
})

var cmdRetrack = idCommand("retrack", func(c *cli, cl *trackified.Client, id string) (any, func(), error) {
	r, err := cl.Trackings.Retrack(c.ctx, id)
	return r, func() { fmt.Fprintf(c.out, "Queued a fresh check of %s\n", id) }, err
})

var cmdStop = idCommand("stop", func(c *cli, cl *trackified.Client, id string) (any, func(), error) {
	r, err := cl.Trackings.Delete(c.ctx, id)
	return r, func() { fmt.Fprintf(c.out, "Stopped tracking %s\n", id) }, err
})

var cmdDetect = idCommand("detect", func(c *cli, cl *trackified.Client, number string) (any, func(), error) {
	r, err := cl.Carriers.Detect(c.ctx, number)
	return r, func() {
		table(c.out, "CARRIER\tCONFIDENCE\tREASON", func(tw io.Writer) {
			for _, x := range r.Candidates {
				fmt.Fprintf(tw, "%s\t%.2f\t%s\n", x.Carrier, x.Confidence, x.Reason)
			}
		})
	}, err
})

func cmdList(c *cli, args []string) error {
	fs := c.flags("list")
	status := fs.String("status", "", "filter by status (e.g. in_transit, delivered)")
	carrier := fs.String("carrier", "", "filter by carrier")
	all := fs.Bool("all", false, "fetch every page")
	limit := fs.Int("limit", 50, "page size (1-100)")
	if _, err := parse(fs, args, 0); err != nil {
		return err
	}
	client, err := c.client()
	if err != nil {
		return err
	}
	p := &trackified.TrackingListParams{ListParams: trackified.ListParams{Limit: *limit}, Status: trackified.TrackingStatus(*status), Carrier: *carrier}
	var items []trackified.Tracking
	total := 0
	if *all {
		for t, err := range client.Trackings.All(c.ctx, p) {
			if err != nil {
				return err
			}
			items = append(items, t)
		}
		total = len(items)
	} else {
		l, err := client.Trackings.List(c.ctx, p)
		if err != nil {
			return err
		}
		items, total = l.Data, l.Pagination.Total
	}
	if items == nil {
		items = []trackified.Tracking{}
	}
	return c.emit(items, func() {
		table(c.out, "ID\tNUMBER\tCARRIER\tSTATUS\tUPDATED", func(tw io.Writer) {
			for _, t := range items {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", t.ID, t.TrackingNumber, orDash(t.Carrier), t.Status, t.UpdatedAt.Local().Format(time.DateTime))
			}
		})
		if len(items) < total {
			fmt.Fprintf(c.out, "\nShowing %d of %d. Use --all for every page.\n", len(items), total)
		}
	})
}

func eventKey(e trackified.TrackingEvent) string {
	return e.OccurredAt + "|" + string(e.Status) + "|" + e.Description + "|" + e.Location
}

func cmdWatch(c *cli, args []string) error {
	fs := c.flags("watch")
	interval := fs.Duration("interval", time.Minute, "how often to poll")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	if *interval < 5*time.Second {
		return usageError("--interval must be at least 5s")
	}
	client, err := c.client()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for first := true; ; first = false {
		t, err := client.Trackings.Get(c.ctx, pos[0])
		if err != nil {
			return err
		}
		if first && !c.json {
			printTracking(c.out, t, false)
			fmt.Fprintln(c.out)
		}
		// Events come newest first; print new ones oldest first.
		for i := len(t.Events) - 1; i >= 0; i-- {
			e := t.Events[i]
			if seen[eventKey(e)] {
				continue
			}
			seen[eventKey(e)] = true
			if c.json {
				if err := c.emit(e, nil); err != nil {
					return err
				}
			} else {
				printEvent(c.out, e)
			}
		}
		if t.Status == trackified.StatusDelivered || t.Status == trackified.StatusReturned {
			if !c.json {
				fmt.Fprintf(c.out, "\n%s: %s\n", t.TrackingNumber, t.Status)
			}
			return nil
		}
		select {
		case <-c.ctx.Done():
			return nil
		case <-time.After(*interval):
		}
	}
}

func cmdCarriers(c *cli, args []string) error {
	pos, err := parse(c.flags("carriers"), args, -1)
	if err != nil {
		return err
	}
	if len(pos) > 1 {
		return usageError("expected at most one search term")
	}
	client, err := c.client()
	if err != nil {
		return err
	}
	if len(pos) == 1 {
		r, err := client.Carriers.Catalog(c.ctx, &trackified.CatalogParams{Query: pos[0]})
		if err != nil {
			return err
		}
		return c.emit(r.Data, func() {
			table(c.out, "ID\tNAME\tCOUNTRY\tSUPPORT", func(tw io.Writer) {
				for _, x := range r.Data {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", x.ID, x.DisplayName, orDash(x.Country), x.Support)
				}
			})
		})
	}
	var all []trackified.Carrier
	for x, err := range client.Carriers.ListAutoPaging(c.ctx, &trackified.ListParams{Limit: 100}).All() {
		if err != nil {
			return err
		}
		all = append(all, x)
	}
	return c.emit(all, func() {
		table(c.out, "ID\tNAME", func(tw io.Writer) {
			for _, x := range all {
				fmt.Fprintf(tw, "%s\t%s\n", x.ID, x.DisplayName)
			}
		})
	})
}

func cmdWebhooks(c *cli, args []string) error {
	if len(args) == 0 {
		return usageError("expected list, add, rm or test")
	}
	sub, args := args[0], args[1:]
	fs := c.flags("webhooks " + sub)
	events := fs.String("events", "", "comma-separated event types for add (default: all)")
	want := map[string]int{"list": 0, "add": 1, "rm": 1, "test": 1}
	n, ok := want[sub]
	if !ok {
		return usageError("unknown subcommand " + sub + "; expected list, add, rm or test")
	}
	pos, err := parse(fs, args, n)
	if err != nil {
		return err
	}
	client, err := c.client()
	if err != nil {
		return err
	}
	switch sub {
	case "list":
		var all []trackified.Webhook
		for w, err := range client.Webhooks.ListAutoPaging(c.ctx, nil).All() {
			if err != nil {
				return err
			}
			all = append(all, w)
		}
		if all == nil {
			all = []trackified.Webhook{}
		}
		return c.emit(all, func() {
			table(c.out, "ID\tURL\tENABLED\tEVENTS", func(tw io.Writer) {
				for _, w := range all {
					ev := make([]string, len(w.EventTypes))
					for i, e := range w.EventTypes {
						ev[i] = string(e)
					}
					fmt.Fprintf(tw, "%s\t%s\t%t\t%s\n", w.ID, w.URL, w.Enabled, strings.Join(ev, ","))
				}
			})
		})
	case "add":
		p := &trackified.CreateWebhookParams{URL: pos[0]}
		if *events != "" {
			for _, e := range strings.Split(*events, ",") {
				p.EventTypes = append(p.EventTypes, trackified.WebhookEventType(strings.TrimSpace(e)))
			}
		}
		w, err := client.Webhooks.Create(c.ctx, p)
		if err != nil {
			return err
		}
		return c.emit(w, func() {
			fmt.Fprintf(c.out, "Added %s -> %s\nSigning secret (shown once, store it now): %s\n", w.ID, w.URL, w.Secret)
		})
	case "rm":
		if err := client.Webhooks.Delete(c.ctx, pos[0]); err != nil {
			return err
		}
		return c.emit(map[string]any{"deleted": true, "id": pos[0]}, func() { fmt.Fprintf(c.out, "Removed %s\n", pos[0]) })
	default: // test
		r, err := client.Webhooks.Test(c.ctx, pos[0])
		if err != nil {
			return err
		}
		return c.emit(r, func() {
			fmt.Fprintf(c.out, "x-tracking-signature: %s\n", r.Signature)
			c.json = true
			_ = c.emit(r.Body, nil)
		})
	}
}

func cmdUsage(c *cli, args []string) error {
	if _, err := parse(c.flags("usage"), args, 0); err != nil {
		return err
	}
	client, err := c.client()
	if err != nil {
		return err
	}
	u, err := client.Usage.Get(c.ctx)
	if err != nil {
		return err
	}
	return c.emit(u, func() {
		fmt.Fprintf(c.out, "Plan:        %s\nPeriod:      %s to %s\nTrackings:   %d of %d used (%d left)\nRate limit:  %d/min\nWebhooks:    %d deliveries\n",
			u.Plan, u.PeriodStart.Format(time.DateOnly), u.PeriodEnd.Format(time.DateOnly),
			u.TrackingsUsed, u.TrackingsLimit, u.TrackingsRemaining, u.RateLimitPerMinute, u.WebhookDeliveries)
		if len(u.CarrierVolume) > 0 {
			fmt.Fprintln(c.out)
			table(c.out, "CARRIER\tTRACKINGS", func(tw io.Writer) {
				for _, v := range u.CarrierVolume {
					fmt.Fprintf(tw, "%s\t%d\n", v.Carrier, v.Count)
				}
			})
		}
	})
}

func cmdVerifyWebhook(c *cli, args []string) error {
	fs := c.flags("verify-webhook")
	secret := fs.String("secret", os.Getenv("TRACKIFIED_WEBHOOK_SECRET"), "webhook signing secret (default $TRACKIFIED_WEBHOOK_SECRET)")
	header := fs.String("header", "", "value of the x-tracking-signature header")
	bodyFile := fs.String("body-file", "", "file holding the raw request body (- for stdin)")
	tolerance := fs.Duration("tolerance", trackified.DefaultTolerance, "allowed clock difference")
	if _, err := parse(fs, args, 0); err != nil {
		return err
	}
	if *secret == "" || *header == "" || *bodyFile == "" {
		return usageError("--secret, --header and --body-file are required")
	}
	var body []byte
	var err error
	if *bodyFile == "-" {
		body, err = io.ReadAll(c.stdin)
	} else {
		body, err = os.ReadFile(*bodyFile)
	}
	if err != nil {
		return err
	}
	ok := trackified.VerifyWebhook(body, *header, *secret, *tolerance)
	if err := c.emit(map[string]bool{"valid": ok}, func() {
		if ok {
			fmt.Fprintln(c.out, "Signature valid")
		} else {
			fmt.Fprintln(c.out, "Signature INVALID")
		}
	}); err != nil {
		return err
	}
	if !ok {
		return errSilent
	}
	return nil
}

func cmdVersion(c *cli, args []string) error {
	if _, err := parse(c.flags("version"), args, 0); err != nil {
		return err
	}
	return c.emit(map[string]string{"version": version, "sdk": trackified.Version}, func() {
		fmt.Fprintf(c.out, "trackified %s (trackified-go %s)\n", version, trackified.Version)
	})
}

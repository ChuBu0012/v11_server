package bot

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// DefaultRingDuration is the safety auto-off duration for the device alarm (30 seconds).
const DefaultRingDuration = 30 * time.Second

// DefaultAlarmSoundPath is the system ringtone used for Find My Phone on Android.
const DefaultAlarmSoundPath = "/system/media/audio/alarms/Clock_Alert.ogg"

// RingController safely manages device audio, vibration, heads-up lockscreen notification,
// and safety auto-off timer for the Find My Phone (/ring) feature on Android/Termux.
type RingController struct {
	mu           sync.Mutex
	isRinging    bool
	autoOffTimer *time.Timer
	autoOffTime  time.Time
	autoOffDur   time.Duration
	soundPath    string
	runner       CommandRunner
}

// RingOption configures RingController.
type RingOption func(*RingController)

// WithRingDuration sets a custom auto-off duration.
func WithRingDuration(d time.Duration) RingOption {
	return func(r *RingController) {
		if d > 0 {
			r.autoOffDur = d
		}
	}
}

// WithRingSoundPath sets a custom sound file path.
func WithRingSoundPath(path string) RingOption {
	return func(r *RingController) {
		if path != "" {
			r.soundPath = path
		}
	}
}

// WithRingCommandRunner overrides the system command runner.
func WithRingCommandRunner(runner CommandRunner) RingOption {
	return func(r *RingController) {
		if runner != nil {
			r.runner = runner
		}
	}
}

// NewRingController creates a new RingController instance.
func NewRingController(opts ...RingOption) *RingController {
	c := &RingController{
		autoOffDur: DefaultRingDuration,
		soundPath:  DefaultAlarmSoundPath,
		runner:     DefaultCommandRunner,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// IsRinging returns true if the device is currently sounding an alarm.
func (c *RingController) IsRinging() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isRinging
}

// RemainingDuration returns how much time remains before auto-off.
func (c *RingController) RemainingDuration() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.isRinging {
		return 0
	}
	rem := time.Until(c.autoOffTime)
	if rem < 0 {
		return 0
	}
	return rem
}

// Ring turns on max volume, plays alarm sound in loop, vibrates, and posts a Heads-Up lockscreen notification.
func (c *RingController) Ring(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 1. Maximize stream volumes
	_ = c.runner(ctx, "termux-volume", "stream", "alarm", "15")
	_ = c.runner(ctx, "termux-volume", "stream", "music", "15")

	// 2. Start audio playback
	if err := c.runner(ctx, "termux-media-player", "play", c.soundPath); err != nil {
		log.Printf("[bot:ring] termux-media-player play failed (%v), falling back to TTS alert", err)
		_ = c.runner(ctx, "termux-tts-speak", "Pocket Sentinel อยู่ตรงนี้ครับ")
	}

	// 3. Pulse vibration
	_ = c.runner(ctx, "termux-vibrate", "-d", "3000", "-f")

	// 4. Post Heads-Up lockscreen notification with direct dismissal action
	notiAction := "termux-media-player stop && termux-notification-remove sentinel_ring"
	_ = c.runner(ctx, "termux-notification",
		"--id", "sentinel_ring",
		"--ongoing",
		"--priority", "high",
		"--title", "🔔 Pocket Sentinel กำลังส่งเสียง",
		"--content", "แตะปุ่มเพื่อปิดเสียงทันที หรือปัดการแจ้งเตือนทิ้ง",
		"--button1", "🔕 ปิดเสียง",
		"--button1-action", notiAction,
		"--on-delete", "termux-media-player stop",
	)

	c.isRinging = true
	c.cancelTimerLocked()

	c.autoOffTime = time.Now().Add(c.autoOffDur)
	c.autoOffTimer = time.AfterFunc(c.autoOffDur, func() {
		c.autoOff()
	})

	return fmt.Sprintf("🔔 <b>ส่งเสียงเรียกโทรศัพท์แล้ว</b>\n⏳ เสียงจะหยุดอัตโนมัติใน %v\n💡 แตะปุ่มบนหน้าจอโทรศัพท์หรือกดปุ่มด้านล่างเพื่อปิดเสียง", c.autoOffDur), nil
}

// Stop stops the alarm, removes the notification, and resets state.
func (c *RingController) Stop(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cancelTimerLocked()

	// Stop media player
	_ = c.runner(ctx, "termux-media-player", "stop")

	// Remove notification
	_ = c.runner(ctx, "termux-notification-remove", "sentinel_ring")

	wasRinging := c.isRinging
	c.isRinging = false

	if wasRinging {
		return "🔕 <b>ปิดเสียงโทรศัพท์เรียบร้อยแล้ว</b>", nil
	}
	return "ℹ️ โทรศัพท์ไม่ได้กำลังส่งเสียงอยู่", nil
}

func (c *RingController) autoOff() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isRinging {
		return
	}

	bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = c.runner(bgCtx, "termux-media-player", "stop")
	_ = c.runner(bgCtx, "termux-notification-remove", "sentinel_ring")

	c.isRinging = false
	log.Printf("[bot:ring] Safety auto-off timer expired; stopped ring alarm.")
}

func (c *RingController) cancelTimerLocked() {
	if c.autoOffTimer != nil {
		c.autoOffTimer.Stop()
		c.autoOffTimer = nil
	}
}

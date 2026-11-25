package mindmap

import "strings"

// Glyph returns the character a mind map draws for an icon, given the
// classes of ::icon(...), as "fa fa-book" or "mdi mdi-skull-outline", or
// "" when it has none.
//
// Mermaid leaves icons to icon fonts (Font Awesome, Material Design Icons)
// that the page showing the diagram loads. A diagram drawn natively has no
// such fonts, so the common icons show as emoji instead, before the label.
func Glyph(classes string) string {
	for _, c := range strings.Fields(classes) {
		name, ok := iconName(c)
		if !ok {
			continue
		}
		if g := glyphs[name]; g != "" {
			return g
		}
	}
	return ""
}

// iconName returns the name of the icon a class names, without the
// prefix of its set, or false when the class names no icon (fa, fas,
// fa-solid, fa-lg and so on).
func iconName(class string) (string, bool) {
	class = strings.ToLower(class)
	for _, prefix := range []string{"fa-", "mdi-", "bi-", "ti-", "icon-"} {
		if name, ok := strings.CutPrefix(class, prefix); ok {
			return name, !iconModifiers[name]
		}
	}
	return "", false
}

// iconModifiers are the classes of Font Awesome that style an icon rather
// than name one.
var iconModifiers = map[string]bool{
	"solid": true, "regular": true, "light": true, "thin": true, "duotone": true, "brands": true, "sharp": true,
	"xs": true, "sm": true, "lg": true, "xl": true, "2x": true, "3x": true, "4x": true, "5x": true,
	"fw": true, "spin": true, "pulse": true, "border": true, "inverse": true,
}

// glyphs maps the names of common icons of Font Awesome and Material
// Design Icons to emoji.
var glyphs = map[string]string{}

func init() {
	for glyph, names := range map[string]string{
		"📖":  "book book-open book-open-variant open-book",
		"🔖":  "bookmark",
		"🎓":  "graduation-cap user-graduate school-outline",
		"🏫":  "school",
		"💡":  "lightbulb lightbulb-on lightbulb-outline idea",
		"🧠":  "brain",
		"❤️": "heart heart-outline",
		"💓":  "heartbeat heart-pulse",
		"⭐":  "star star-outline",
		"🏠":  "home house",
		"👤":  "user account person",
		"👥":  "users user-group people-group account-group",
		"⚙️": "cog cogs gear gears",
		"🔧":  "wrench",
		"🛠️": "tools screwdriver-wrench hammer-wrench",
		"🔨":  "hammer",
		"💻":  "laptop laptop-code code terminal computer desktop monitor",
		"🗄️": "database",
		"🖥️": "server",
		"☁️": "cloud cloud-outline",
		"🌐":  "globe globe-americas earth web",
		"📧":  "envelope email mail",
		"📞":  "phone",
		"📱":  "mobile mobile-alt mobile-screen cellphone",
		"📷":  "camera",
		"🖼️": "image",
		"🎵":  "music",
		"🎬":  "video film movie",
		"📅":  "calendar calendar-alt calendar-days",
		"⏰":  "clock clock-outline alarm",
		"🔔":  "bell",
		"🔒":  "lock",
		"🔓":  "unlock lock-open",
		"🔑":  "key",
		"🛡️": "shield shield-alt shield-halved",
		"✅":  "check check-circle circle-check",
		"❌":  "times xmark close",
		"⚠️": "exclamation-triangle triangle-exclamation warning alert",
		"ℹ️": "info info-circle circle-info information",
		"❓":  "question question-circle circle-question help",
		"🔍":  "search magnifying-glass magnify",
		"📊":  "chart-bar chart-line chart-pie chart-simple chart-column poll",
		"📈":  "chart-line-up arrow-trend-up trending-up",
		"💵":  "dollar-sign money-bill money cash",
		"🪙":  "coins",
		"🛒":  "shopping-cart cart cart-shopping",
		"🚀":  "rocket",
		"🚩":  "flag",
		"🏆":  "trophy",
		"🎯":  "bullseye target",
		"🧩":  "puzzle-piece puzzle",
		"💬":  "comment comments chat message",
		"📄":  "file file-alt file-lines file-document",
		"📁":  "folder folder-open",
		"✏️": "pen pencil pencil-alt edit",
		"🎨":  "paint-brush palette",
		"🌿":  "leaf",
		"🌳":  "tree",
		"🌱":  "seedling sprout",
		"☀️": "sun",
		"🌙":  "moon",
		"🔥":  "fire fire-alt",
		"⚡":  "bolt lightning-bolt flash",
		"💧":  "water tint droplet",
		"🛏️": "bed",
		"🏃":  "running person-running run",
		"🏋️": "dumbbell",
		"🍎":  "apple apple-alt apple-whole food-apple",
		"🍴":  "utensils food silverware",
		"☕":  "coffee mug-hot",
		"🚗":  "car",
		"✈️": "plane airplane",
		"🚌":  "bus",
		"🚆":  "train",
		"🚲":  "bicycle bike",
		"🚢":  "ship",
		"🗺️": "map",
		"📍":  "map-marker map-marker-alt location-dot map-pin",
		"🏥":  "hospital",
		"🩺":  "stethoscope",
		"🤖":  "robot",
		"🔗":  "link",
		"⬇️": "download",
		"⬆️": "upload",
		"🔄":  "sync rotate refresh",
		"🗑️": "trash trash-alt trash-can delete",
		"🎁":  "gift",
		"🤝":  "handshake",
		"💼":  "briefcase",
		"🏢":  "building",
		"🏭":  "industry factory",
		"🧪":  "flask",
		"⚛️": "atom",
		"💀":  "skull skull-outline",
		"🐛":  "bug",
		"📶":  "wifi",
		"🔋":  "battery battery-full",
		"🔌":  "plug",
		"👁️": "eye",
		"👍":  "thumbs-up",
		"😊":  "smile face-smile",
		"🐾":  "paw",
		"🐕":  "dog",
		"🐈":  "cat",
		"🎮":  "gamepad",
		"📰":  "newspaper",
		"📣":  "bullhorn",
		"🏷️": "tag tags",
		"📦":  "box package",
		"🚚":  "truck",
		"📋":  "list tasks list-check clipboard clipboard-list",
	} {
		for _, n := range strings.Fields(names) {
			glyphs[n] = glyph
		}
	}
}

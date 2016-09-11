package irc

const (
	FlagOps UserChanFlag = '@'

	// TODO: the rfc specifies this, but afaik doesn't explain what it means. Need to investigate.
	FlagWHAT UserChanFlag = '+'
)

const (
	ChanPublic  ChanPrivacy = '='
	ChanPrivate ChanPrivacy = '*'
	ChanSecret  ChanPrivacy = '@'
)

type Usermode int
type ChanPrivacy rune
type UserChanFlag rune

type UserChanInfo struct {
	Flag   UserChanFlag
	Prefix UserPrefix
}

type UserPrefix struct {
	Nick, User, Host string
}

type ClientPrivmsg struct {
	To string
}

type ClientNick struct {
	Nick string
}

type ClientUser struct {
	UserPrefix
}

type ServerPrivmsg struct {
	To   string
	From UserPrefix
}

type ServerNick struct {
	Nick string
	User UserPrefix
}

type ServerJoin struct {
	Channel string
	User    UserPrefix
}

type RplNamereply struct {
	ChannelName String
	ChanPrivacy
	Users []UserChanInfo
}

func ParseUserPrefix(msg *irc.Message) (UserPrefix, error) {
	return UserPrefix{}, nil
}

func ParseClientMessage(msg *irc.Message) (interface{}, error) {
	switch msg.Command {
	case "NICK":
		if err := checkParamCount(1, len(msg.Params)); err != nil {
			return err
		}
		return ClientNick{Nick: msg.Params[0]}, nil
	}
}

func ParseServerMessage(msg *irc.Messasge) (interface{}, error) {
	switch msg.Command {
	case "NICK":
		if err := checkParamCount(1, len(msg.Params)); err != nil {
			return err
		}
		prefix, err := ParseUserPreifx(msg.Prefix)
		if err != nil {
			return NestedError{Ctx: "Paring Server NICK", Err: err}
		}
		return ServerNick{Nick: msg.Params[0], User: prefix}, nil
	}
}

func checkParamCount(wanted, got int) error {
	if wanted < got {
		return NotEnoughParams{Wanted: wanted, Got: got}
	} else {
		return nil
	}
}

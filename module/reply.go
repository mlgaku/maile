package module

import (
	"encoding/json"
	"github.com/mlgaku/back/db"
	"github.com/mlgaku/back/service"
	. "github.com/mlgaku/back/types"
	"gopkg.in/mgo.v2/bson"
	"math"
	"regexp"
	"strings"
	"time"
)

type Reply struct {
	db db.Reply

	service.Di
}

// 添加新回复
func (r *Reply) New() Value {
	user := r.Ses().Get("user").(*db.User)

	reply := db.NewReply(r.Req().Body, "i")
	reply.Author = user.Id

	topic := new(db.Topic).Find(reply.Topic)

	// 添加回复
	r.db.Add(reply)

	// 更新最后回复
	topic.UpdateReply(reply.Topic, user.Name)

	// 回复人不是主题作者时添加通知
	if reply.Author != topic.Author {
		new(db.Notice).Add(&db.Notice{
			Type:       1,
			Date:       time.Now(),
			Master:     topic.Author,
			User:       user.Name,
			TopicID:    reply.Topic,
			TopicTitle: topic.Title,
		})
	}

	// 通知被at的人
	r.handleAt(user.Name, topic, reply)

	// 更新余额
	conf := r.Conf()
	if conf.Reward.NewReply != 0 {
		new(db.User).Inc(reply.Author, "balance", conf.Reward.NewReply)
		new(db.Bill).Add(&db.Bill{
			Msg:    topic.Title,
			Type:   2,
			Date:   time.Now(),
			Number: conf.Reward.NewReply,
			Master: reply.Author,
		})
	}

	r.Ps().Publish(&Prot{Mod: "reply", Act: "list"})
	r.Ps().Publish(&Prot{Mod: "notice", Act: "list"})
	return &Succ{}
}

// 编辑回复内容
func (r *Reply) Edit() Value {
	user := r.Di.Ses().Get("user").(*db.User)

	reply := db.NewReply(r.Di.Req().Body, "b")
	oldReply := reply.Find(reply.Id)

	// 内容和原来一样
	if reply.Content = strings.Trim(reply.Content, " "); reply.Content == oldReply.Content {
		return &Succ{}
	}

	r.db.UpdateContent(reply.Id, reply.Content)

	// 不是修改自己的回复时添加通知
	if oldReply.Author != user.Id {
		typ := 6
		if reply.Content == "" { // 回复被删除
			typ = 7
		}

		new(db.Notice).Add(&db.Notice{
			Type:       uint64(typ),
			Date:       time.Now(),
			Master:     oldReply.Author,
			User:       user.Name,
			TopicID:    oldReply.Topic,
			TopicTitle: new(db.Topic).Find(oldReply.Topic).Title,
			ReplyID:    oldReply.Id,
		})

		r.Ps().Publish(&Prot{Mod: "notice", Act: "list"})
	}

	r.Ps().Publish(&Prot{Mod: "reply", Act: "list"})
	return &Succ{}
}

// 获取回复列表
func (r *Reply) List() Value {
	var s struct {
		Page  int
		Topic bson.ObjectId
	}
	if err := json.Unmarshal(r.Req().Body, &s); err != nil {
		return &Fail{Msg: err.Error()}
	}

	total := math.Ceil(float64(r.db.Count(s.Topic)) / 20)
	if s.Page == -1 {
		s.Page = int(total)
	}

	return &Succ{Data: M{
		"per":   20,
		"page":  s.Page,
		"total": total,
		"list":  r.db.Paginate(s.Topic, s.Page, 20),
	}}
}

// 处理 At
func (*Reply) handleAt(name string, topic *db.Topic, reply *db.Reply) {
	match := regexp.MustCompile(`@[a-zA-Z0-9]+`).FindAllString(reply.Content, 5)
	if match == nil {
		return
	}

	for k, v := range match {
		match[k] = strings.TrimLeft(v, "@")
	}

	user := new(db.User).FindByNameMany(match)

	for k, v := range user {
		// 跳过@自己
		if k == name {
			continue
		}

		new(db.Notice).Add(&db.Notice{
			Type:       2,
			Date:       time.Now(),
			Master:     v.Id,
			User:       name,
			TopicID:    reply.Topic,
			TopicTitle: topic.Title,
		})
	}
}

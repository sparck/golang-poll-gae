package poll

import (
	"log"
	"time"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"golang.org/x/net/context"

	"google.golang.org/appengine/datastore"
)

func init() {
	api, err := endpoints.RegisterService(PollApi{}, "sparck", "v1", "polls api", true)
	if err != nil {
		log.Fatal(err)
	}

	info := api.MethodByName("List").Info()
	info.Name, info.HTTPMethod, info.Path = "list", "GET", "polls"
	info = api.MethodByName("Add").Info()
	info.Name, info.HTTPMethod, info.Path = "add", "POST", "polls"
	info = api.MethodByName("Get").Info()
	info.Name, info.HTTPMethod, info.Path = "get", "GET", "polls/{uid}"
	info = api.MethodByName("Put").Info()
	info.Name, info.HTTPMethod, info.Path = "put", "PUT", "polls/{uid}"
	info = api.MethodByName("Delete").Info()
	info.Name, info.HTTPMethod, info.Path = "delete", "DELETE", "polls/{uid}"
	info = api.MethodByName("Vote").Info()
	info.Name, info.HTTPMethod, info.Path = "vote", "POST", "polls/{uid}/vote/{answer}"

	endpoints.HandleHTTP()
}

type PollApi struct{}

type ListReqPolls struct {
	Poll
	Limit int          `json:"limit" endpoints:"d=10,min=1,max=50"`
	Page  *QueryMarker `json:"cursor"`
}

type ListPolls struct {
	Polls []Poll       `json:"polls"`
	Next  *QueryMarker `json:"next,omitempty"`
}

func (PollApi) List(c context.Context, r *ListReqPolls) (*ListPolls, error) {

	list := &ListPolls{Polls: make([]Poll, 0, r.Limit)}

	q := datastore.NewQuery("Poll").Limit(r.Limit)
	if r.Page != nil {
		q = q.Start(r.Page.Cursor)
	}

	var iter *datastore.Iterator
	for iter = q.Run(c); ; {
		var p Poll
		key, err := iter.Next(&p)
		if err == datastore.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		p.UID = key
		list.Polls = append(list.Polls, p)
	}

	cur, err := iter.Cursor()
	if err != nil {
		return nil, err
	}

	list.Next = &QueryMarker{cur}
	return list, nil
}

type AddRequest struct {
	Question string   `endpoints:"req"`
	Answers  []Answer `endpoints:"req"`
}

func (PollApi) Add(c context.Context, r *AddRequest) (*Poll, error) {
	if err := checkReferer(c); err != nil {
		return nil, err
	}

	p := &Poll{
		Question: r.Question,
		Answers:  r.Answers,
		Created:  time.Now(),
		Updated:  time.Now(),
	}

	if err := p.IsValid(); err != nil {
		return nil, endpoints.NewBadRequestError(err.Error())
	}

	k := datastore.NewIncompleteKey(c, "Poll", nil)
	k, err := datastore.Put(c, k, p)
	if err != nil {
		return nil, err
	}
	p.UID = k

	return p, nil
}

type GetRequest struct {
	UID *datastore.Key `json:"uid" endpoints:"req"`
}

func (PollApi) Get(c context.Context, r *GetRequest) (*Poll, error) {
	var p Poll

	if err := datastore.Get(c, r.UID, &p); err == datastore.ErrNoSuchEntity {
		return nil, endpoints.NewNotFoundError("Poll not found")
	} else if err != nil {
		return nil, endpoints.NewBadRequestError("Id not valid")
	}

	p.UID = r.UID
	return &p, nil
}

type PutRequest struct {
	UID      *datastore.Key `json:"uid"`
	Question string         `endpoints:"req"`
	Answers  []Answer       `endpoints:"req"`
}

func (PollApi) Put(c context.Context, r *PutRequest) (*Poll, error) {
	if err := checkReferer(c); err != nil {
		return nil, err
	}

	var p Poll
	err := datastore.RunInTransaction(c, func(c context.Context) error {
		if err := datastore.Get(c, r.UID, &p); err == datastore.ErrNoSuchEntity {
			return endpoints.NewNotFoundError("Poll not found")
		} else if err != nil {
			return endpoints.NewBadRequestError("Id not valid")
		}

		p.Question = r.Question
		p.Answers = r.Answers

		_, err := datastore.Put(c, r.UID, &p)

		return err
	}, nil)

	p.UID = r.UID

	return &p, err
}

type DeleteRequest struct {
	UID *datastore.Key `json:"uid" endpoints:"req"`
}

func (PollApi) Delete(c context.Context, r *DeleteRequest) error {

	if err := datastore.Delete(c, r.UID); err == datastore.ErrNoSuchEntity {
		return endpoints.NewNotFoundError("Poll not found")
	} else if err != nil {
		return endpoints.NewBadRequestError("Id not valid")
	}

	return nil
}

type VoteRequest struct {
	UID    *datastore.Key `json:"uid" endpoints:"req"`
	Answer int            `json:"answer" endpoints:"req"`
}

func (PollApi) Vote(c context.Context, r *VoteRequest) error {
	if err := checkReferer(c); err != nil {
		return err
	}

	return datastore.RunInTransaction(c, func(c context.Context) error {
		var p Poll
		if err := datastore.Get(c, r.UID, &p); err == datastore.ErrNoSuchEntity {
			return endpoints.NewNotFoundError("Poll not found")
		} else if err != nil {
			return err
		}

		if count := len(p.Answers); count > r.Answer {
			p.Answers[r.Answer].Votes++
			p.Updated = time.Now()
		} else {
			return endpoints.NewBadRequestError("Answer not found")
		}

		_, err := datastore.Put(c, r.UID, &p)
		return err
	}, nil)
}

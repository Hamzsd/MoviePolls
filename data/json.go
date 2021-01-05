package data

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zorchenhimer/MoviePolls/common"
)

type jsonMovie struct {
	Id             int
	Name           string
	Links          []int
	Description    string
	Remarks        string
	Duration       string
	Rating         float32
	CycleAddedId   int
	CycleWatchedId int
	Removed        bool
	Approved       bool
	Poster         string
	AddedBy        int
	Tags           []int
}

func (j *jsonConnector) newJsonMovie(movie *common.Movie) jsonMovie {
	currentCycle := j.currentCycle()
	cycleId := 0
	if currentCycle != nil {
		cycleId = currentCycle.Id
	}

	cycleWatched := 0
	if movie.CycleWatched != nil {
		cycleWatched = movie.CycleWatched.Id
	}

	tags := []int{}
	if movie.Tags != nil {
		for _, tag := range movie.Tags {
			tags = append(tags, tag.Id)
		}
	}

	links := []int{}
	if movie.Links != nil {
		for _, link := range movie.Links {
			links = append(links, link.Id)
		}
	}

	id := j.nextMovieId()

	jm := jsonMovie{
		Id:             id,
		Name:           movie.Name,
		Links:          links,
		Description:    movie.Description,
		Remarks:        movie.Remarks,
		Duration:       movie.Duration,
		Rating:         movie.Rating,
		CycleAddedId:   cycleId,
		CycleWatchedId: cycleWatched,
		Removed:        movie.Removed,
		Approved:       movie.Approved,
		Poster:         movie.Poster,
		Tags:           tags,
	}

	if movie.AddedBy != nil {
		jm.AddedBy = movie.AddedBy.Id
	}

	if movie.Tags != nil {
		tags := []int{}
		for _, tag := range movie.Tags {
			tags = append(tags, tag.Id)
		}
		jm.Tags = tags
	}

	if movie.Links != nil {
		links := []int{}
		for _, link := range movie.Links {
			links = append(links, link.Id)
		}
		jm.Links = links
	}

	return jm
}

type jsonVote struct {
	UserId  int
	MovieId int
	CycleId int
}

type jsonCycle struct {
	Id         int
	PlannedEnd *time.Time
	Ended      *time.Time
	Watched    []int
}

type jsonLink struct {
	Id       int
	IsSource bool
	Type     string
	Url      string
}

func (j *jsonConnector) newJsonCycle(cycle *common.Cycle) jsonCycle {
	watched := []int{}
	if cycle.Watched != nil {
		for _, movie := range cycle.Watched {
			watched = append(watched, movie.Id)
		}
	}

	return jsonCycle{
		Id:         cycle.Id,
		PlannedEnd: cycle.PlannedEnd,
		Ended:      cycle.Ended,
		Watched:    watched,
	}
}

type jsonConnector struct {
	filename string `json:"-"`
	lock     *sync.RWMutex

	Cycles map[int]jsonCycle
	Movies map[int]jsonMovie
	Users  map[int]*common.User
	Votes  []jsonVote
	Tags   map[int]*common.Tag
	Links  map[int]*common.Link

	//Settings Configurator
	Settings map[string]configValue

	l *common.Logger
}

func init() {
	register("json", func(connStr string, l *common.Logger) (DataConnector, error) {

		if _, err := os.Stat(filepath.Dir(connStr)); os.IsNotExist(err) {
			os.MkdirAll(filepath.Dir(connStr), 0777)
		}
		_, err := os.OpenFile(connStr, os.O_RDONLY|os.O_CREATE, 0777)
		if err != nil {
			fmt.Printf("error creating file %s:%v", connStr, err)
			return nil, err
		}
		dc, err := newJsonConnector(connStr, l)
		return DataConnector(dc), err
	})
}

func newJsonConnector(filename string, l *common.Logger) (*jsonConnector, error) {
	if common.FileExists(filename) {
		return loadJson(filename, l)
	}

	j := &jsonConnector{
		filename: filename,
		lock:     &sync.RWMutex{},
		Settings: map[string]configValue{},

		Cycles: map[int]jsonCycle{},
		Movies: map[int]jsonMovie{},
		Users:  map[int]*common.User{},
		Tags:   map[int]*common.Tag{},
		Links:  map[int]*common.Link{},
		l:      l,
	}

	return j, j.save()
}

func loadJson(filename string, l *common.Logger) (*jsonConnector, error) {
	raw, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	data := &jsonConnector{}
	err = json.Unmarshal(raw, data)
	if err != nil {
		return nil, fmt.Errorf("Unable to read JSON data: %v", err)
	}

	data.filename = filename
	data.lock = &sync.RWMutex{}
	data.l = l

	if data.Settings == nil {
		data.Settings = make(map[string]configValue)
	}

	if data.Users == nil {
		data.Users = make(map[int]*common.User)
	}

	if data.Votes == nil {
		data.Votes = []jsonVote{}
	}

	if data.Movies == nil {
		data.Movies = make(map[int]jsonMovie)
	}

	if data.Cycles == nil {
		data.Cycles = make(map[int]jsonCycle)
	}

	if data.Tags == nil {
		data.Tags = make(map[int]*common.Tag)
	}

	if data.Links == nil {
		data.Links = make(map[int]*common.Link)
	}

	return data, nil
}

func (j *jsonConnector) save() error {
	raw, err := json.MarshalIndent(j, "", " ")
	if err != nil {
		return fmt.Errorf("Unable to marshal JSON data: %v", err)
	}

	err = ioutil.WriteFile(j.filename, raw, 0777)
	if err != nil {
		return fmt.Errorf("Unable to write JSON data: %v", err)
	}

	return nil
}

/*
   On determining the current cycle.

   Should the current cycle have an end date?
   If so, this would be the automatic end date for the cycle.
   If not, only the current cycle would have an end date, which would define
   the current cycle as the cycle without an end date.
*/
func (j *jsonConnector) currentCycle() *common.Cycle {
	for _, c := range j.Cycles {
		if c.Ended == nil {
			return j.cycleFromJson(c)
		}
	}
	return nil
}

func (j *jsonConnector) cycleFromJson(cycle jsonCycle) *common.Cycle {
	c := &common.Cycle{
		Id:         cycle.Id,
		PlannedEnd: cycle.PlannedEnd,
		Ended:      cycle.Ended,
	}

	if cycle.PlannedEnd != nil {
		t := (*cycle.PlannedEnd).Round(time.Second)
		c.PlannedEnd = &t
	}
	if cycle.Ended != nil {
		t := (*cycle.Ended).Round(time.Second)
		c.Ended = &t
	}

	if cycle.Watched != nil {
		movies := []*common.Movie{}
		for _, m := range cycle.Watched {
			movies = append(movies, j.findMovie(m))
		}
		c.Watched = movies
	}

	return c
}

func (j *jsonConnector) jsonFromCycle(cycle *common.Cycle) jsonCycle {
	c := jsonCycle{
		Id:         cycle.Id,
		PlannedEnd: cycle.PlannedEnd,
		Ended:      cycle.Ended,
	}

	if cycle.PlannedEnd != nil {
		t := (*cycle.PlannedEnd).Round(time.Second)
		c.PlannedEnd = &t
	}

	if cycle.Ended != nil {
		t := (*cycle.Ended).Round(time.Second)
		c.Ended = &t
	}

	if cycle.Watched != nil {
		movies := []int{}
		for _, m := range cycle.Watched {
			movies = append(movies, m.Id)
		}
		c.Watched = movies
	}
	return c
}

func (j *jsonConnector) jsonFromVote(vote *common.Vote) jsonVote {
	if vote.User == nil || vote.Movie == nil || vote.CycleAdded == nil {
		panic("Invalid vote.  Missing user, move, or cycle")
	}

	return jsonVote{
		UserId:  vote.User.Id,
		MovieId: vote.Movie.Id,
		CycleId: vote.CycleAdded.Id,
	}
}

func (j *jsonConnector) GetCurrentCycle() (*common.Cycle, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	return j.currentCycle(), nil
}

func (j *jsonConnector) GetCycle(id int) (*common.Cycle, error) {
	for _, c := range j.Cycles {
		if c.Id == id {
			return j.cycleFromJson(c), nil
		}
	}

	return nil, fmt.Errorf("Cycle not found with ID %d", id)
}

func (j *jsonConnector) AddCycle(plannedEnd *time.Time) (int, error) {
	j.lock.Lock()
	defer j.lock.Unlock()

	if j.Cycles == nil {
		j.Cycles = map[int]jsonCycle{}
	}

	if plannedEnd != nil {
		t := (*plannedEnd).Round(time.Second)
		plannedEnd = &t
	}

	c := jsonCycle{
		Id:         j.nextCycleId(),
		PlannedEnd: plannedEnd,
	}

	j.Cycles[c.Id] = c

	return c.Id, j.save()
}

func (j *jsonConnector) AddOldCycle(c *common.Cycle) (int, error) {
	j.lock.Lock()
	defer j.lock.Unlock()

	if j.Cycles == nil {
		j.Cycles = map[int]jsonCycle{}
	}

	c.Id = j.nextCycleId()
	if c.PlannedEnd != nil {
		t := (*c.PlannedEnd).Round(time.Second)
		c.PlannedEnd = &t
	}
	if c.Ended != nil {
		t := (*c.Ended).Round(time.Second)
		c.Ended = &t
	}

	j.Cycles[c.Id] = j.jsonFromCycle(c)
	return c.Id, j.save()
}

func (j *jsonConnector) nextCycleId() int {
	highest := 0
	for _, c := range j.Cycles {
		if c.Id > highest {
			highest = c.Id
		}
	}
	return highest + 1
}

func (j *jsonConnector) nextMovieId() int {
	highest := 0
	for _, m := range j.Movies {
		if m.Id >= highest {
			highest = m.Id
		}
	}
	return highest + 1
}

func (j *jsonConnector) AddMovie(movie *common.Movie) (int, error) {
	j.lock.Lock()
	defer j.lock.Unlock()

	if j.Movies == nil {
		j.Movies = map[int]jsonMovie{}
	}

	if j.Tags == nil {
		j.Tags = map[int]*common.Tag{}
	}

	if j.Links == nil {
		j.Links = map[int]*common.Link{}
	}

	m := j.newJsonMovie(movie)
	j.Movies[m.Id] = m

	return m.Id, j.save()
}

func (j *jsonConnector) GetMovie(id int) (*common.Movie, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	movie := j.findMovie(id)
	if movie == nil {
		return nil, fmt.Errorf("Movie with ID %d not found.", id)
	}

	movie.Votes = j.findVotes(movie)
	return movie, nil
}

func (j *jsonConnector) GetActiveMovies() ([]*common.Movie, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	movies := []*common.Movie{}

	for _, m := range j.Movies {
		mov, _ := j.GetMovie(m.Id)
		if mov != nil && m.CycleWatchedId == 0 {
			movies = append(movies, mov)
		}
	}

	return movies, nil
}

type sortableCycle []jsonCycle

func (s sortableCycle) Len() int { return len(s) }

// sort in reverse
// FIXME: sort by date instead of ID
func (s sortableCycle) Less(i, j int) bool { return s[i].Id > s[j].Id }
func (s sortableCycle) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func (j *jsonConnector) GetPastCycles(start, end int) ([]*common.Cycle, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	past := sortableCycle{}
	for _, cycle := range j.Cycles {
		if cycle.Ended != nil {
			past = append(past, cycle)
		}
	}

	sort.Sort(past)
	filtered := []*common.Cycle{}
	idx := start
	for i := 0; i < end && i+idx < len(past); i++ {
		f := past[idx+i]
		f.Watched = []int{}

		j.l.Debug("[GetPastCycles] finding watched movies for cycle %d", f.Id)
		for _, movie := range j.Movies {
			if movie.CycleWatchedId == f.Id {
				j.l.Debug("  found movie with ID %d", movie.Id)
				f.Watched = append(f.Watched, movie.Id)
			}
		}

		filtered = append(filtered, j.cycleFromJson(f))
	}

	return filtered, nil
}

func (j *jsonConnector) movieFromJson(jMovie jsonMovie) *common.Movie {
	user := j.findUser(jMovie.AddedBy)

	tags := []*common.Tag{}

	links := []*common.Link{}

	for _, id := range jMovie.Tags {
		t, ok := j.Tags[id]
		if ok {
			tags = append(tags, t)
		}
	}

	for _, id := range jMovie.Links {
		l, ok := j.Links[id]
		if ok {
			links = append(links, l)
		}
	}

	movie := &common.Movie{
		Id:          jMovie.Id,
		Name:        jMovie.Name,
		Description: jMovie.Description,
		Duration:    jMovie.Duration,
		Rating:      jMovie.Rating,
		Remarks:     jMovie.Remarks,
		Removed:     jMovie.Removed,
		Approved:    jMovie.Approved,
		//CycleAdded:   j.findCycle(jMovie.CycleAddedId),
		//CycleWatched: j.findCycle(jMovie.CycleWatchedId),
		Links:   links,
		Poster:  jMovie.Poster,
		AddedBy: user,
		Tags:    tags,
	}

	return movie
}

func (j *jsonConnector) GetMoviesFromCycle(id int) ([]*common.Movie, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	watched := j.findCycle(id)
	if watched == nil {
		return nil, fmt.Errorf("Cycle with ID %d not found", id)
	}

	movies := []*common.Movie{}
	for _, movie := range j.Movies {
		if movie.CycleWatchedId == id {
			m := j.movieFromJson(movie)

			m.CycleWatched = watched
			m.CycleAdded = j.findCycle(movie.CycleAddedId)

			movies = append(movies, j.movieFromJson(movie))
		}
	}

	return movies, nil
}

// UserLogin returns a user if the given username and password match a user.
func (j *jsonConnector) UserLogin(name, hashedPw string) (*common.User, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	name = strings.ToLower(name)
	for _, user := range j.Users {
		if strings.ToLower(user.Name) == name {
			if hashedPw == user.Password {
				return user, nil
			}
			j.l.Info("Bad password for user %s\n", name)
			return nil, fmt.Errorf("Invalid login credentials")
		}
	}
	j.l.Info("User with name %s not found\n", name)
	return nil, fmt.Errorf("Invalid login credentials")
}

// Get the total number of users
func (j *jsonConnector) GetUserCount() int {
	j.lock.RLock()
	defer j.lock.RUnlock()

	return len(j.Users)
}

func (j *jsonConnector) GetUsers(start, count int) ([]*common.User, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	uids := []int{}
	for _, u := range j.Users {
		uids = append(uids, u.Id)
	}

	sort.Ints(uids)

	ulist := []*common.User{}
	for i := 0; i < len(uids) && len(ulist) <= count; i++ {
		id := uids[i]
		if id < start {
			continue
		}

		u := j.findUser(id)
		if u != nil {
			ulist = append(ulist, u)
		}
	}

	return ulist, nil
}

func (j *jsonConnector) GetUser(userId int) (*common.User, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	u := j.findUser(userId)
	if u == nil {
		return nil, fmt.Errorf("User not found with ID %d", userId)
	}
	return u, nil
}

func (j *jsonConnector) GetUserVotes(userId int) ([]*common.Movie, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	votes := []*common.Movie{}
	for _, v := range j.Votes {
		if v.UserId == userId {
			mov := j.findMovie(v.MovieId)
			if mov != nil {
				votes = append(votes, mov)
			}
		}
	}

	return votes, nil
}

func (j *jsonConnector) GetUserMovies(userId int) ([]*common.Movie, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	movies := []*common.Movie{}
	for _, m := range j.Movies {
		if m.AddedBy == userId {
			mov := j.findMovie(m.Id)
			if mov != nil {
				movies = append(movies, mov)
			}
		}
	}

	return movies, nil
}

func sortCycles(cycles map[int]jsonCycle) []jsonCycle {
	slc := []jsonCycle{}
	for _, c := range cycles {
		slc = append(slc, c)
	}
	sorted := sortableCycle(slc)
	sort.Sort(sorted)

	return sorted
}

// Find votes for currently active movies and remove the ones that have been
// added more than `age` cycles ago.  Do not remove votes from movies that have
// been watched.
func (j *jsonConnector) DecayVotes(age int) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	// Older cycles will have a lower ID
	sorted := sortCycles(j.Cycles)

	// Get the ID of the cycle that's at the age boundary.
	idLimit := 0
	for i, cycle := range sorted {
		if i >= age {
			idLimit = cycle.Id
			break
		}
	}

	newVotes := []jsonVote{} // non-decayed votes
	mcache := map[int]bool{} // movie watched true/false

	for _, vote := range j.Votes {
		// Figure out if the movie has been watched
		var watched bool
		if w, ok := mcache[vote.MovieId]; ok {
			watched = w
		} else {
			m := j.findMovie(vote.MovieId)
			watched = (m.CycleWatched != nil)
			mcache[vote.MovieId] = watched
		}

		// If the movie hasn't been watched and the vote was added below the
		// cycle ID limit, decay the vote.
		if !watched && vote.CycleId < idLimit {
			j.l.Debug("Decaying vote for movie ID %d", vote.MovieId)
		} else {
			newVotes = append(newVotes, vote)
		}
	}

	// Remember to save the new vote list
	j.Votes = newVotes
	return j.save()
}

func (j *jsonConnector) nextUserId() int {
	highest := 0
	for _, u := range j.Users {
		if u.Id > highest {
			highest = u.Id
		}
	}
	return highest + 1
}

func (j *jsonConnector) AddUser(user *common.User) (int, error) {
	j.lock.Lock()
	defer j.lock.Unlock()

	if j.Users == nil {
		j.Users = map[int]*common.User{}
	}

	if _, exists := j.Users[user.Id]; exists {
		return 0, fmt.Errorf("User already exists with ID %d", user.Id)
	}

	name := strings.ToLower(user.Name)
	for _, u := range j.Users {
		if strings.ToLower(u.Name) == name {
			return 0, fmt.Errorf("User already exists with name %s", user.Name)
		}
	}

	user.Id = j.nextUserId()

	j.Users[user.Id] = user
	return user.Id, j.save()
}

func (j *jsonConnector) AddVote(userId, movieId int) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	user := j.findUser(userId)
	if user == nil {
		return fmt.Errorf("User not found with ID %d", userId)
	}

	movie := j.findMovie(movieId)
	if movie == nil {
		return fmt.Errorf("Movie not found with ID %d", movieId)
	}

	if movie.CycleWatched != nil {
		return fmt.Errorf("Movie has already been watched")
	}

	if movie.Removed {
		return fmt.Errorf("Movie has been removed by a mod or admin")
	}

	cc := j.currentCycle()
	if cc == nil {
		return fmt.Errorf("No cycle currently active")
	}

	j.Votes = append(j.Votes, jsonVote{userId, movieId, cc.Id})
	return j.save()
}

func (j *jsonConnector) AddTag(tag *common.Tag) (int, error) {
	j.lock.Lock()
	defer j.lock.Unlock()

	if tag.Name == "" {
		return 0, fmt.Errorf("Name cannot be empty")
	}

	//duplicate check
	for id, jtag := range j.Tags {
		if strings.ToLower(tag.Name) == strings.ToLower(jtag.Name) {
			j.l.Debug("Tag '%v' is already in the database with id: %v", tag.Name, id)
			return id, nil
		}
	}

	id := j.nextTagId()

	tag.Id = id

	j.Tags[id] = tag
	return id, j.save()
}

func (j *jsonConnector) FindTag(name string) (int, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	name = strings.ToLower(name)

	for id, tag := range j.Tags {
		if strings.ToLower(tag.Name) == name {
			return id, nil
		}
	}
	return 0, fmt.Errorf("No tag found with name: %s", name)
}

func (j *jsonConnector) GetTag(id int) *common.Tag {
	j.lock.RLock()
	defer j.lock.RUnlock()
	return j.Tags[id]
}

func (j *jsonConnector) DeleteTag(id int) {
	j.lock.Lock()
	defer j.lock.Unlock()

	delete(j.Tags, id)
}

func (j *jsonConnector) nextTagId() int {
	highest := 0
	for _, t := range j.Tags {
		if t.Id >= highest {
			highest = t.Id
		}
	}
	return highest + 1
}

func (j *jsonConnector) AddLink(link *common.Link) (int, error) {
	j.lock.Lock()
	defer j.lock.Unlock()

	if link.Url == "" {
		return 0, fmt.Errorf("Link url cannot be empty")
	}

	if link.Type == "" {
		return 0, fmt.Errorf("Link type cannot be empty")
	}

	//duplicate check
	for id, jlink := range j.Links {
		if strings.ToLower(link.Url) == strings.ToLower(jlink.Url) {
			j.l.Debug("Link '%v' is already in the database with id: %v", link.Url, id)
			return id, nil
		}
	}

	id := j.nextLinkId()

	link.Id = id

	j.Links[id] = link
	return id, j.save()
}

func (j *jsonConnector) FindLink(url string) (int, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	url = strings.ToLower(url)

	for id, link := range j.Links {
		if strings.ToLower(link.Url) == url {
			return id, nil
		}
	}
	return 0, fmt.Errorf("No link found with url: %s", url)
}

func (j *jsonConnector) GetLink(id int) *common.Link {
	j.lock.RLock()
	defer j.lock.RUnlock()
	return j.Links[id]
}

func (j *jsonConnector) DeleteLink(id int) {
	j.lock.Lock()
	defer j.lock.Unlock()

	delete(j.Links, id)
}

func (j *jsonConnector) nextLinkId() int {
	highest := 0
	for _, l := range j.Links {
		if l.Id >= highest {
			highest = l.Id
		}
	}
	return highest + 1
}

func (j *jsonConnector) requireApproval() bool {
	// ignore errors here.  "false" is default.
	val, _ := j.GetCfgBool("RequireApproval", false)
	return val
}

func (j *jsonConnector) DeleteVote(userId, movieId int) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	mov := j.findMovie(movieId)
	if mov.CycleWatched != nil {
		return fmt.Errorf("Cannot remove vote for watched movie.")
	}

	found := false
	newVotes := []jsonVote{}
	for _, v := range j.Votes {
		if v.UserId == userId && v.MovieId == movieId {
			found = true
		} else {
			newVotes = append(newVotes, v)
		}
	}

	if !found {
		return fmt.Errorf("Vote not found for current cycle")
	}
	j.Votes = newVotes
	return j.save()
}

func (j *jsonConnector) CheckMovieExists(title string) (bool, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	clean := common.CleanMovieName(title)
	for _, m := range j.Movies {
		if clean == common.CleanMovieName(m.Name) {
			return true, nil
		}
	}
	return false, nil
}

func (j *jsonConnector) CheckUserExists(name string) (bool, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	lc := strings.ToLower(name)
	for _, user := range j.Users {
		if lc == strings.ToLower(user.Name) {
			return true, nil
		}
	}
	return false, nil
}

/* Find */

func (j *jsonConnector) findMovie(id int) *common.Movie {
	if id == 0 {
		return nil
	}

	m, ok := j.Movies[id]
	if ok {
		movie := j.movieFromJson(m)
		movie.CycleWatched = j.findCycle(m.CycleWatchedId)
		movie.CycleAdded = j.findCycle(m.CycleAddedId)

		return movie
	}

	j.l.Info("findMovie() not found with ID %d\n", id)
	return nil
}

func (j *jsonConnector) findCycle(id int) *common.Cycle {
	if id == 0 {
		return nil
	}

	c, ok := j.Cycles[id]
	if ok {
		cycle := &common.Cycle{
			Id: c.Id,
		}
		if c.PlannedEnd != nil {
			t := (*c.PlannedEnd).Round(time.Second)
			cycle.PlannedEnd = &t
		}

		if c.Ended != nil {
			t := (*c.Ended).Round(time.Second)
			cycle.Ended = &t
		}
		return cycle
	}
	return nil
}

func (j *jsonConnector) findVotes(movie *common.Movie) []*common.Vote {
	votes := []*common.Vote{}
	for _, v := range j.Votes {
		if v.MovieId == movie.Id {
			votes = append(votes, &common.Vote{
				Movie:      movie,
				CycleAdded: j.findCycle(v.CycleId),
				User:       j.findUser(v.UserId),
			})
		}
	}

	return votes
}

func (j *jsonConnector) findUser(id int) *common.User {
	for _, u := range j.Users {
		if u.Id == id {
			return u
		}
	}
	return nil
}

/* Update */

func (j *jsonConnector) UpdateUser(user *common.User) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	j.Users[user.Id] = user
	return j.save()
}

func (j *jsonConnector) UpdateMovie(movie *common.Movie) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	m := j.newJsonMovie(movie)
	m.Id = movie.Id
	j.Movies[m.Id] = m

	return j.save()
}

func (j *jsonConnector) UpdateCycle(cycle *common.Cycle) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	j.Cycles[cycle.Id] = j.jsonFromCycle(cycle)
	return j.save()
}

func (j *jsonConnector) UserVotedForMovie(userId, movieId int) (bool, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	for _, v := range j.Votes {
		if v.MovieId == movieId && v.UserId == userId {
			return true, nil
		}
	}

	return false, nil
}

// Configuration stuff
type cfgValType int

const (
	CVT_STRING cfgValType = iota
	CVT_INT
	CVT_BOOL
)

type configValue struct {
	Type  cfgValType
	Value interface{}
}

func (v configValue) String() string {
	t := ""
	switch v.Type {
	case CVT_STRING:
		t = "string"
		break
	case CVT_INT:
		t = "int"
		break
	case CVT_BOOL:
		t = "bool"
		break
	}

	return fmt.Sprintf("configValue{Type:%s Value:%v}", t, v.Value)
}

func (j *jsonConnector) GetCfgString(key, value string) (string, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	val, ok := j.Settings[key]
	if !ok {
		return value, nil
		//return "", fmt.Errorf("Setting with key %q does not exist", key)
	}

	switch val.Type {
	case CVT_STRING:
		return val.Value.(string), nil
	case CVT_INT:
		return "", fmt.Errorf("%q is an INT key, not a STRING key", key)
	case CVT_BOOL:
		return "", fmt.Errorf("%q is a BOOL key, not a STRING key", key)
	default:
		return "", fmt.Errorf("Unknown type %d", val.Type)
	}
}

func (j *jsonConnector) GetCfgInt(key string, value int) (int, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	val, ok := j.Settings[key]
	if !ok {
		return value, nil
		//return 0, fmt.Errorf("Setting with key %q does not exist", key)
	}

	switch val.Type {
	case CVT_STRING:
		return 0, fmt.Errorf("%q is a STRING key, not an INT key", key)
	case CVT_INT:
		if val, ok := val.Value.(int); ok {
			return val, nil
		}
		if val, ok := val.Value.(float64); ok {
			return int(val), nil
		}
		return 0, fmt.Errorf("Unknown number type for %s", key)
	case CVT_BOOL:
		return 0, fmt.Errorf("%q is a BOOL key, not an INT key", key)
	default:
		return 0, fmt.Errorf("Unknown type %d", val.Type)
	}
}

func (j *jsonConnector) GetCfgBool(key string, value bool) (bool, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	val, ok := j.Settings[key]
	if !ok {
		return value, nil
		//return false, fmt.Errorf("Setting with key %q does not exist", key)
	}

	switch val.Type {
	case CVT_STRING:
		bval, err := strconv.ParseBool(val.Value.(string))
		if err != nil {
			return false, fmt.Errorf("Bool parse error: %s", err)
		}
		return bval, nil
	case CVT_INT:
		return false, fmt.Errorf("%q is an INT key, not a BOOL key", key)
	case CVT_BOOL:
		v, ok := val.Value.(bool)
		return (ok && v), nil
	default:
		return false, fmt.Errorf("Unknown type %d", val.Type)
	}
}

func (j *jsonConnector) SetCfgString(key, value string) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	j.Settings[key] = configValue{CVT_STRING, value}

	return j.save()
}

func (j *jsonConnector) SetCfgInt(key string, value int) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	j.Settings[key] = configValue{CVT_INT, value}

	return j.save()
}

func (j *jsonConnector) SetCfgBool(key string, value bool) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	j.Settings[key] = configValue{CVT_BOOL, value}

	return j.save()
}

func (j *jsonConnector) DeleteCfgKey(key string) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	delete(j.Settings, key)

	return j.save()
}

func (j *jsonConnector) DeleteUser(userId int) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	if _, exists := j.Users[userId]; !exists {
		return fmt.Errorf("User with ID %d does not exist", userId)
	}

	delete(j.Users, userId)
	return j.save()
}

func (j *jsonConnector) PurgeUser(userId int) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	count := 0
	newVotes := []jsonVote{}
	for _, vote := range j.Votes {
		if vote.UserId != userId {
			newVotes = append(newVotes, vote)
		} else {
			count++
		}
	}
	j.Votes = newVotes
	j.l.Info("Purged %d votes", count)

	delete(j.Users, userId)
	return j.save()
}

func (j *jsonConnector) SearchMovieTitles(query string) ([]*common.Movie, error) {
	j.lock.RLock()
	defer j.lock.RUnlock()

	found := []*common.Movie{}
	query = strings.ToLower(query)
	words := strings.Split(query, " ")

	for _, movie := range j.Movies {
		ok := true
		for _, word := range words {
			if !strings.Contains(strings.ToLower(movie.Name), word) {
				ok = false
				break
			}
		}

		if ok {
			m := j.findMovie(movie.Id)
			m.Votes = j.findVotes(m)
			found = append(found, m)
		}
	}

	return found, nil
}

func (j *jsonConnector) DeleteCycle(cycleId int) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	if _, exists := j.Cycles[cycleId]; !exists {
		return fmt.Errorf("Cycle with ID %d does not exist!", cycleId)
	}

	delete(j.Cycles, cycleId)
	return nil
}

func (j *jsonConnector) RemoveMovie(movieId int) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	// Verify movie is active (don't allow deleting watched movies)
	if mov, ok := j.Movies[movieId]; ok {
		if mov.CycleWatchedId != 0 {
			return fmt.Errorf("Cannot remove movie, it has already been watched.")
		}
	}

	// Delete votes
	newVotes := []jsonVote{}
	for _, vote := range j.Votes {
		if vote.MovieId != movieId {
			newVotes = append(newVotes, vote)
		}
	}
	j.Votes = newVotes

	// Delete movie
	delete(j.Movies, movieId)

	return j.save()
}

func (j *jsonConnector) DeleteMovie(movieId int) error {
	j.lock.Lock()
	defer j.lock.Unlock()

	if _, exists := j.Movies[movieId]; !exists {
		return fmt.Errorf("Movie with ID %d does not exist!", movieId)
	}

	delete(j.Movies, movieId)
	return nil
}

func (j *jsonConnector) Test_GetUserVotes(userId int) ([]*common.Vote, error) {
	votes := []*common.Vote{}
	for _, vote := range j.Votes {
		if vote.UserId != userId {
			continue
		}
		u := j.findUser(vote.UserId)
		m := j.findMovie(vote.MovieId)
		c := j.findCycle(vote.CycleId)

		votes = append(votes, &common.Vote{CycleAdded: c, Movie: m, User: u})
	}
	return votes, nil
}

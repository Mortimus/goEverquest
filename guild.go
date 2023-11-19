package everquest

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func GetRecentRosterDump(path string, guildName string) (string, error) {
	var files []string

	err := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if strings.HasPrefix(d.Name(), guildName) {
			files = append(files, d.Name())
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(files) <= 0 {
		return "", errors.New("cannot find a recent raid dump")
	}
	return files[len(files)-1], nil // return last file - should be latest
}

type Guild struct {
	Members []GuildMember
}

type GuildMember struct {
	Name                string    // Character Name
	Level               int       // Character Level
	Class               string    // Player class ex: Necromancer
	Rank                string    // Guild Rank Name
	Alt                 bool      // Is this character flagged as an alt
	LastOnline          time.Time // Last time this character was online
	Zone                string    // Zone this character is currently in
	PublicNote          string    // Public Note
	PersonalNote        string    // Personal Note - assumed
	TributeStatus       bool      // Tribute status on or off
	TrophyTributeStatus bool      // Trophy Tribute Status on or off
	Donations           int       // total donations
	LastDonation        time.Time // Last date of donation
	PublicNote2         string    // Seems to be the public note again not sure why
	PersonalNote2       string    // Probably Personal Note again
}

// LoadFromPath takes a standard everquest guild dump and loads it into a struct
func (guild *Guild) LoadFromPath(path string, Err *log.Logger) error {
	// Open the file
	tsvfile, err := os.Open(path)
	if err != nil {
		Err.Println("Couldn't open the tsv file", err)
		return errors.New("could not open the tsv file at " + path)
	}
	defer tsvfile.Close()

	// Parse the file
	r := csv.NewReader(tsvfile)
	r.Comma = '\t'

	// Iterate through the records
	for {
		// Read each record from csv
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}
		level, err := strconv.Atoi(record[1])
		if err != nil {
			Err.Printf("Error converting level to int - Level: %s Name: %s\n", record[1], record[0])
			continue
		}
		var alt bool
		if record[4] == "A" {
			alt = true
		}
		format := "01/02/06"
		lastOnline, err := time.Parse(format, record[5])
		if err != nil {
			Err.Printf("Error converting last_online to time - Time: %s\n", record[5])
			continue
		}
		var tributeStatus bool
		if record[9] == "on" {
			tributeStatus = true
		}
		var trophyTributeStatus bool
		if record[10] == "on" {
			trophyTributeStatus = true
		}
		donations, err := strconv.Atoi(strings.Replace(record[11], ",", "", -1))
		if err != nil {
			Err.Printf("Error converting donations to int - Donation: %s Name: %s\n", record[11], record[0])
			continue
		}
		var lastDonation time.Time
		if record[12] != "" {
			lastDonation, err = time.Parse(format, record[12])
			if err != nil {
				Err.Printf("Error converting last_donation to time - Time: %s\n", record[12])
				continue

			}
		}

		guildMember := GuildMember{
			Name:                record[0],
			Level:               level,
			Class:               record[2],
			Rank:                record[3],
			Alt:                 alt,
			LastOnline:          lastOnline,
			Zone:                record[6],
			PublicNote:          record[7],
			PersonalNote:        record[8],
			TributeStatus:       tributeStatus,
			TrophyTributeStatus: trophyTributeStatus,
			Donations:           donations,
			LastDonation:        lastDonation,
			PublicNote2:         record[13],
			PersonalNote2:       record[14],
		}
		guild.Members = append(guild.Members, guildMember)
	}
	return nil
}

func (member *GuildMember) HasRank(ranks []string) bool {
	for _, rank := range ranks {
		if rank == member.Rank {
			return true
		}
	}
	return false
}

func (member *GuildMember) IsClass(classes []string) bool {
	for _, class := range classes {
		if class == member.Class {
			return true
		}
	}
	return false
}

func NewGuildMembers(master, new Guild) []GuildMember {
	masterList := make(map[string]interface{}, len(master.Members))
	var results []GuildMember
	for _, masterMember := range master.Members {
		masterList[masterMember.Name] = nil
	}
	for _, newMember := range new.Members {
		if _, ok := masterList[newMember.Name]; !ok {
			results = append(results, newMember)
		}
	}
	return results
}

func MissingGuildMembers(master, new Guild) []GuildMember {
	return NewGuildMembers(new, master)
}

func MergeGuilds(master, new Guild) Guild {
	uniqueGuild := make(map[string]GuildMember)
	for _, mMember := range master.Members {
		uniqueGuild[mMember.Name] = mMember
	}
	for _, nMember := range new.Members {
		uniqueGuild[nMember.Name] = nMember
	}
	// Convert to Guild
	var newGuild Guild
	var newMembers []GuildMember
	for _, fMember := range uniqueGuild {
		newMembers = append(newMembers, fMember)
	}
	newGuild.Members = newMembers
	return newGuild
}

// GetClassCount will return from a guild dump all the members that meet the requested class/level/online/alt/rank requirements specified
func GetClassCount(guild Guild, minLevel int, onlineAfter time.Time, includeAlts bool, ranks []string, classes []string) []GuildMember {
	var results []GuildMember
	for _, mem := range guild.Members {
		if mem.Level >= minLevel && mem.LastOnline.After(onlineAfter) && ((includeAlts && mem.Alt) || !mem.Alt) && mem.HasRank(ranks) && mem.IsClass(classes) {
			results = append(results, mem)
		}
	}
	return results
}

func (guild *Guild) GetMemberByName(name string) (GuildMember, error) {
	for _, member := range guild.Members {
		if member.Name == name {
			return member, nil
		}
	}
	return GuildMember{}, errors.New("could not find member with name: " + name)
}

func (guild *Guild) WriteToPath(path string) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		return err
	}

	datawriter := bufio.NewWriter(file)
	// defer datawriter.Flush()
	defer file.Close()

	for _, member := range guild.Members {
		var isAlt string
		if member.Alt {
			isAlt = "A"
		}
		lastOn := member.LastOnline.Format("01/02/06")
		var tributeStatus string
		if member.TributeStatus {
			tributeStatus = "on"
		} else {
			tributeStatus = "off"
		}
		var trophyTributeStatus string
		if member.TrophyTributeStatus {
			trophyTributeStatus = "on"
		} else {
			trophyTributeStatus = "off"
		}
		lastDonation := member.LastDonation.Format("01/02/06")
		line := fmt.Sprintf("%s\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s", member.Name, member.Level, member.Class, member.Rank, isAlt, lastOn, member.Zone, member.PublicNote, member.PersonalNote, tributeStatus, trophyTributeStatus, member.Donations, lastDonation, member.PublicNote2, member.PersonalNote2)
		_, err = datawriter.WriteString(line + "\n")
		if err != nil {
			return err
		}
		datawriter.Flush()
	}
	return nil
}

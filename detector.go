package main

import (
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
	"unicode"
	"golang.org/x/text/runes"
	"bytes"
	"github.com/lestrrat/go-ngram"
	"github.com/leesper/go_rng"
	"strings"
	"fmt"
	"encoding/xml"
	"unicode/utf8"
	"path/filepath"
	"log"
	"regexp"
	"os"
	"encoding/gob"
	"math/rand"
	"time"
	"sync"
)

const ABSTRACT_WIKI_THRESHOLD int = 100
const ALL_LANGUAGES = "all"

const ALPHA_DEFAULT = 0.5
const ALPHA_WIDTH = 0.05

const BASE_FREQ = 10000;

const MAX_TRIALS = 15
const MAX_ITTERATIONS = 255
const MIN_PROB = 0.000001


type DetectConfig struct {
	XmlPath string
	ProfilePath string
	Profile string
	Languages []string
}

type classData struct {
	Freqs   map[string]float64
	Total   int
}

type Response struct {
	BestLang string						`json:"lang"`
	BestProb float64					`json:"prob"`
	BestStrict bool						`json:"strict"`
	Languages map[string]float64 		`json:"langs"`
	TotalTokens	int						`jsgithub.com/leesper/go_rngon:"tokens_total"`
	TokenProcessed	int					`json:"tokens_processed"`
}

func (c *classData) getProb(gram string) float64 {
	value, ok := c.Freqs[gram]
	if !ok {
		return 0.00000000001
	}
	r := float64(value) / float64(c.Total)
	return r
}

type Detector struct{
	classes map[string]*classData
	config DetectConfig
}

func NewDetector(config DetectConfig) *Detector {
	if len(config.Languages) == 1 && config.Languages[0] == ALL_LANGUAGES {
		config.Languages = getProfileAvailableLanguages(config)
	}

	d := &Detector{
		classes: make(map[string]*classData),
		config: config,
	}

	if _, err := os.Stat(d.config.ProfilePath + "/" + d.config.Profile); !os.IsNotExist(err) {
		for _, lang := range config.Languages {
			d.ReadClassFromFile(lang, config.ProfilePath+"/"+config.Profile)
		}
	}

	return d
}


func (d *Detector) ReadClassFromFile(class string, location string) (err error) {
	fileName := filepath.Join(location, class)
	file, err := os.Open(fileName)

	if err != nil {
		return err
	}

	dec := gob.NewDecoder(file)
	w := new(classData)
	err = dec.Decode(w)

	if len(w.Freqs) > 0 {
		d.classes[class] = w
	}
	return
}

func (d *Detector) SaveProfile(name string) error {
	path := d.config.ProfilePath + "/" + d.config.Profile
	_ = os.Mkdir(path, os.ModePerm)

	if _,ok := d.classes[name];!ok {
		return fmt.Errorf("Class not exists")
	}


	fileName := filepath.Join(path, string(name))
	file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	enc := gob.NewEncoder(file)
	err = enc.Encode(d.classes[name])

	return nil
}

func (d *Detector) SaveProfiles() error {
	path := d.config.ProfilePath + "/" + d.config.Profile
	_ = os.Mkdir(path, os.ModePerm)

	for name, _ := range d.classes {
		fileName := filepath.Join(path, string(name))
		file, err := os.OpenFile(fileName, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
		enc := gob.NewEncoder(file)
		err = enc.Encode(d.classes[name])
	}
	return nil
}

func (d *Detector) GenerateProfileFromWikiXML()  {
	files, err := filepath.Glob(d.config.XmlPath + "/*wiki-latest-abstract.xml")
	if err != nil {
		log.Fatal(err)
	}

	regName := regexp.MustCompile("^" + d.config.XmlPath + "/([a-zA-Z0-9]+)wiki-latest-abstract.xml$")

	var xmlFiles = make(map[string]string)

	for _, f := range files {
		lang := regName.FindStringSubmatch(f)[1]
		if _,ok := d.classes[lang];ok || len(lang) < 2 {
			continue;
		}
		d.classes[lang] = new(classData)
		d.classes[lang].Freqs = make(map[string]float64);
		xmlFiles[lang] = f
	}


	var wg sync.WaitGroup
	for lang, file  := range xmlFiles {
		wg.Add(1)
		go d.processXml(lang, file, &wg)
	}
	wg.Wait()
}

func (d *Detector) processXml(lang string, file string, wg *sync.WaitGroup) {
	defer wg.Done()
	var text string
	var strChan chan string
	strChan = parseXml(file)
	for text = range strChan {
		d.Train(tokenize(text), lang)
	}

	d.ClearFreqsByClass(lang,2)
	d.SaveProfile(lang)
	log.Println("Ready ", lang)
}


func (d *Detector) Train(ngrams []string, class string) {
	for _, gram := range ngrams {
		d.classes[class].Freqs[gram]++
		d.classes[class].Total++
	}
}

func (d *Detector) ClearFreqsByClass(class string, min float64) {
	t := 0
	for word, freq := range d.classes[class].Freqs {
		if freq < min {
			delete(d.classes[class].Freqs, word)
		} else {
			t = t + int(freq)
		}
	}
	d.classes[class].Total = t
}

func (d *Detector) ClearFreqs(min float64) {
	for class, _ := range d.classes {
		d.ClearFreqsByClass(class, min)
	}
}

func (d * Detector) Detect(text string, langs []string, coefficients map[string]float64, maxTrials int, maxIterations uint16) (res Response) {

    res.Languages = make(map[string]float64, len(d.classes))

	filteredLangs := []string{}
	for _, l := range langs {
		if _, ok := d.classes[l]; ok {
			if _, ok := res.Languages[l]; !ok {
				res.Languages[l] = 0;
				filteredLangs = append(filteredLangs, l)
			}
		}
	}
	langs = filteredLangs



	text = string(normalize([]byte(text)))

	tokens := tokenize(text)
	res.TotalTokens = len(tokens)

	if maxTrials < 0 {
		maxTrials = res.TotalTokens
	} else if maxTrials == 0 {
		maxTrials = MAX_TRIALS
	}

	if maxIterations < 1 {
		maxIterations = MAX_ITTERATIONS
	}

	grnd := rng.NewGaussianGenerator(time.Now().UnixNano())
	alpha := ALPHA_DEFAULT;

	tempScores := make(map[string]map[int]float64, len(d.classes))
	bases := d.getBaseProbs(langs)

	trial := int(0)

	tokenRandIterator := rand.Perm(len(tokens))

	for trial = 0; trial < maxTrials; trial++ {
		alpha = alpha + grnd.Gaussian(0.0, 1.0) * ALPHA_WIDTH;
		weight := alpha / BASE_FREQ

		if res.TokenProcessed >= len(tokenRandIterator) { break; }

		for _, name := range langs {
			if _, ok := tempScores[name]; !ok { tempScores[name] = make(map[int]float64) }
			tempScores[name][trial] = bases[name]
		}

		iterationCounter := uint16(0)
		for ; res.TokenProcessed < len(tokenRandIterator); res.TokenProcessed++ {
			for _, name := range langs {
				tempScores[name][trial] *= weight + d.classes[name].getProb(tokens[tokenRandIterator[res.TokenProcessed]])
			}
			if (iterationCounter % 5) == 0 {
				if iterationCounter >= maxIterations { break }
			}
			iterationCounter++
		}
	}

	for i := int(0); i < trial; i++ {
		sum := float64(0)
		for _, name := range langs {
			if coef, ok := coefficients[name]; ok {
				tempScores[name][i] *= coef;
			}
			sum += tempScores[name][i]
		}
		for _, name := range langs {
			if sum > 0  {
				tempScores[name][i] /= sum
			}
		}
	}

	for _, name := range langs {
		for i := int(0); i < trial; i++ {
			res.Languages[name] += tempScores[name][i] / float64(trial)
		}

		if res.Languages[name] < MIN_PROB {
			delete(res.Languages, name)
		}
	}



	res.BestLang, res.BestStrict = findMax(res.Languages)
	res.BestProb = res.Languages[res.BestLang]

	return
}

func (d *Detector) getBaseProbs(langs []string) (baseProbs map[string]float64) {
	n := len(langs)
	baseProbs = make(map[string]float64, n)

	for _, lang := range langs {
		baseProbs[lang] = 1.0 / float64(n)
	}

	return
}

func parseXml(file string) chan string {

	xmlFile, err := os.Open(file)
	if err != nil {
		log.Fatal("Error opening file:",err)
	}

	decoder := xml.NewDecoder(xmlFile)
	outChan := make(chan string,100)

	go func() {
		for {
			t, _ := decoder.Token()
			if t == nil {
				xmlFile.Close()
				close(outChan)
				break
			}
			switch se := t.(type) {
			case xml.StartElement:
				if se.Name.Local == "abstract" {
					data, _ := decoder.Token()
					switch chars := data.(type) {
					case xml.CharData:
						str := string(normalize(chars))
						if utf8.RuneCountInString(str) > ABSTRACT_WIKI_THRESHOLD {
							outChan <- str
						}
					}
				}
			default:
			}
		}
	}()

	return outChan
}


func normalize(in []byte) (out []byte) {
	t := transform.Chain(
		norm.NFD,
		runes.Remove(runes.In(unicode.Mn)),
		runes.Map(func(r rune) rune {
			if unicode.Is(unicode.Mn, r)  {
				return -1
			}

			if !unicode.Is(unicode.Ll, r) &&
				!unicode.Is(unicode.Lu, r) &&
				!unicode.Is(unicode.Lt, r) &&
				!unicode.Is(unicode.Lo, r) &&
				!unicode.Is(unicode.Zs, r) {
				return 32
			}

			return unicode.ToLower(r)
		}),
		norm.NFC,
	)

	out, _, _ = transform.Bytes(t, in)

	out = bytes.TrimSpace(out)

	return out
}

func tokenize(text string) (tokens []string) {

	words := strings.Fields(text)
	var tks []*ngram.Token

	for i := 3; i <= 5; i++ {
		for _, w := range words {
			if utf8.RuneCountInString(w) + 2 < i - 1 {
				continue;
			}
			if i > 1 {
				tks = ngram.NewTokenize(i, fmt.Sprint("_", w, "_")).Tokens()
			} else {
				tks = ngram.NewTokenize(i, w).Tokens()
			}

			for _, t := range tks {
				if t != nil {
					tokens = append(tokens, t.String())
				}
			}
		}
	}

	return tokens
}


func getProfileAvailableLanguages(config DetectConfig) []string {
 	res := make([]string,0)

	files, err := filepath.Glob(config.ProfilePath + "/" + config.Profile + "/*")
	if err != nil {
		log.Fatal(err)
	}
	regName := regexp.MustCompile("^" + config.ProfilePath + "/" + config.Profile + "/([a-zA-Z0-9]+)$")
	for _, f := range files {
		match := regName.FindStringSubmatch(f);
		if len(match) == 2 {
			lang := regName.FindStringSubmatch(f)[1]
			if len(lang) < 2 {
				continue;
			}
			res = append(res, lang)
		}
	}

	return res
}

func findMax(scores map[string]float64) (key string, strict bool) {
	strict = true
	key = ""
	for name, currentScore := range scores {
		if scores[key] < currentScore {
			key = name
			strict = true
		} else if scores[key] == currentScore {
			strict = false
		}
	}
	return
}

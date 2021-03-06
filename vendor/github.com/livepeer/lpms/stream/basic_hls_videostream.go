package stream

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ericxtang/m3u8"
	"github.com/golang/glog"
	"github.com/livepeer/go-livepeer/common"
)

const DefaultMediaPlLen = uint(500)

// const DefaultMediaWinLen = uint(5)
const DefaultSegWaitTime = time.Second * 10
const SegWaitInterval = time.Second

var ErrAddVariant = errors.New("ErrAddVariant")
var ErrAddHLSSegment = errors.New("ErrAddHLSSegment")

//BasicHLSVideoStream is a basic implementation of HLSVideoStream
type BasicHLSVideoStream struct {
	masterPlCache       *m3u8.MasterPlaylist
	variantMediaPlCache map[string]*m3u8.MediaPlaylist //StrmID -> MediaPlaylist
	sqMap               map[string]*HLSSegment
	lockMap             map[string]sync.Locker
	strmID              string
	segWaitTime         time.Duration
	subscriber          func(HLSVideoStream, string, *HLSSegment)
	winSize             uint

	//Keep track of the non-variant stream separately
	strmMediaPlCache *m3u8.MediaPlaylist
	strmLock         sync.Locker
}

func NewBasicHLSVideoStream(strmID string, segWaitTime time.Duration, wSize uint) *BasicHLSVideoStream {
	mpl := m3u8.NewMasterPlaylist()
	pl, _ := m3u8.NewMediaPlaylist(wSize, DefaultMediaPlLen)
	strm := &BasicHLSVideoStream{
		masterPlCache:       mpl,
		variantMediaPlCache: make(map[string]*m3u8.MediaPlaylist),
		sqMap:               make(map[string]*HLSSegment),
		lockMap:             make(map[string]sync.Locker),
		strmID:              strmID,
		segWaitTime:         segWaitTime,
		strmMediaPlCache:    pl,
		strmLock:            &sync.Mutex{},
		winSize:             wSize}

	return strm
}

//SetSubscriber sets the callback function that will be called when a new hls segment is inserted
func (s *BasicHLSVideoStream) SetSubscriber(f func(s HLSVideoStream, strmID string, seg *HLSSegment)) {
	s.subscriber = f
}

//GetStreamID returns the streamID
func (s *BasicHLSVideoStream) GetStreamID() string { return s.strmID }

//GetStreamFormat always returns HLS
func (s *BasicHLSVideoStream) GetStreamFormat() VideoFormat { return HLS }

//GetMasterPlaylist returns the master playlist. It will return nil if no variant has been added.
func (s *BasicHLSVideoStream) GetMasterPlaylist() (*m3u8.MasterPlaylist, error) {
	return s.masterPlCache, nil
}

//GetVariantPlaylist returns the media playlist represented by the streamID
func (s *BasicHLSVideoStream) GetVariantPlaylist(strmID string) (*m3u8.MediaPlaylist, error) {
	if strmID == s.GetStreamID() {
		if s.strmMediaPlCache.Count() < s.winSize {
			return nil, nil
		}
		return s.strmMediaPlCache, nil
	}

	pl, ok := s.variantMediaPlCache[strmID]
	if !ok {
		return nil, ErrNotFound
	}

	// glog.Infof("Variant pl len: %v, %v", pl.Count(), pl)
	if pl.Count() < s.winSize {
		return nil, nil
	}

	return pl, nil
}

//GetHLSSegment gets the HLS segment.  It blocks until something is found, or timeout happens.
func (s *BasicHLSVideoStream) GetHLSSegment(strmID string, segName string) (*HLSSegment, error) {
	seg, ok := s.sqMap[sqMapKey(strmID, segName)]
	if !ok {
		return nil, ErrNotFound
	}
	return seg, nil
}

//AddVariant adds a new variant playlist (and therefore, a new HLS video stream) to the master playlist.
func (s *BasicHLSVideoStream) AddVariant(strmID string, variant *m3u8.Variant) error {
	if variant == nil {
		glog.Errorf("Cannot add nil variant")
		return ErrAddVariant
	}

	_, ok := s.variantMediaPlCache[strmID]
	if ok {
		glog.Errorf("Variant %v already exists", strmID)
		return ErrAddVariant
	}

	for _, v := range s.masterPlCache.Variants {
		if v.Bandwidth == variant.Bandwidth && v.Resolution == variant.Resolution {
			glog.Errorf("Variant with Bandwidth %v and Resolution %v already exists", v.Bandwidth, v.Resolution)
			return ErrAddVariant
		}
	}

	//Append to master playlist
	s.masterPlCache.Append(variant.URI, variant.Chunklist, variant.VariantParams)

	//Add to mediaPLCache
	s.variantMediaPlCache[strmID] = variant.Chunklist

	//Create the "media playlist specific" lock
	s.lockMap[strmID] = &sync.Mutex{}

	return nil
}

//AddHLSSegment adds the hls segment to the right stream
func (s *BasicHLSVideoStream) AddHLSSegment(strmID string, seg *HLSSegment) error {
	if _, ok := s.sqMap[sqMapKey(strmID, seg.Name)]; ok {
		return nil //Already have the seg.
	}
	glog.V(common.VERBOSE).Infof("Adding segment: %v", seg.Name)
	if strmID == s.GetStreamID() {
		s.strmLock.Lock()
		defer s.strmLock.Unlock()
		if err := s.strmMediaPlCache.AppendSegment(&m3u8.MediaSegment{SeqId: seg.SeqNo, Duration: seg.Duration, URI: seg.Name}); err != nil {
			glog.Errorf("Error appending segment %v: %v", seg.Name, err)
			return ErrAddHLSSegment
		}
		if s.strmMediaPlCache.Count() > s.winSize {
			s.strmMediaPlCache.Remove()
		}
		s.sqMap[sqMapKey(strmID, seg.Name)] = seg
		if s.subscriber != nil {
			s.subscriber(s, strmID, seg)
		}
		return nil
	}

	lock, ok := s.lockMap[strmID]
	if !ok {
		return ErrNotFound
	}
	lock.Lock()
	defer lock.Unlock()

	//Add segment to media playlist
	pl, ok := s.variantMediaPlCache[strmID]
	if !ok {
		return ErrNotFound
	}
	pl.AppendSegment(&m3u8.MediaSegment{SeqId: seg.SeqNo, Duration: seg.Duration, URI: seg.Name})
	if pl.Count() > s.winSize {
		pl.Remove()
	}

	//Add to buffer
	s.sqMap[sqMapKey(strmID, seg.Name)] = seg

	//Call subscriber
	if s.subscriber != nil {
		s.subscriber(s, strmID, seg)
	}

	return nil
}

func (s BasicHLSVideoStream) String() string {
	return fmt.Sprintf("StreamID: %v, Type: %v, len: %v", s.GetStreamID(), s.GetStreamFormat(), len(s.sqMap))
}

func sqMapKey(strmID, segName string) string {
	return fmt.Sprintf("%v_%v", strmID, segName)
}

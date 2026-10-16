package main

import (
	"hash/fnv"
	"math"
	"time"
)

// Readings are made up, but the same for the same moment: a value is a
// function of the machine, the metric and the time. The history written
// at the start and what the fake agents send afterwards therefore join
// without a seam.

// seed names a series of noise.
func seed(parts ...string) uint64 {
	h := fnv.New64a()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return h.Sum64()
}

// mix is splitmix64's finalizer: a well-spread hash of x.
func mix(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

// white is noise in [-1, 1) that changes with every k.
func white(s uint64, k int64) float64 {
	return float64(mix(s^uint64(k)*0x2545f4914f6cdd1d)>>11)/(1<<53)*2 - 1
}

// smooth is noise in [-1, 1) that drifts over period seconds.
func smooth(s uint64, t, period int64) float64 {
	k := t / period
	f := float64(t-k*period) / float64(period)
	w := (1 - math.Cos(f*math.Pi)) / 2
	return white(s, k)*(1-w) + white(s, k+1)*w
}

// traffic is how busy people keep the machines, from 0 to 1: least at
// 4 in the morning, most in the afternoon, less at weekends.
func traffic(t time.Time) float64 {
	t = t.Local()
	h := float64(t.Hour()) + float64(t.Minute())/60
	v := math.Pow(0.5-0.5*math.Cos(2*math.Pi*(h-4)/24), 1.3)
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		v *= 0.55
	}
	return v
}

// inWindow says whether t falls between from and to past local midnight.
func inWindow(t time.Time, from, to time.Duration) bool {
	t = t.Local()
	midnight := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	d := t.Sub(midnight)
	return d >= from && d < to
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

func ptr[T any](v T) *T { return &v }

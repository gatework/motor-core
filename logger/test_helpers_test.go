package logger

func mustLogrus(opts ...Option) *Logrus {
	log, err := NewLogrusLogger(opts...)
	if err != nil {
		panic(err)
	}
	return log
}

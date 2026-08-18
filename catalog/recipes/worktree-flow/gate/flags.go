package main

type stringListFlag struct {
	values []string
}

func (f *stringListFlag) String() string {
	if f == nil {
		return ""
	}
	return ""
}

func (f *stringListFlag) Set(value string) error {
	f.values = append(f.values, value)
	return nil
}

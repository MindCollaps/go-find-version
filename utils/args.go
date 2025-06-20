package utils

type Args struct {
	GitUrl             string `arg:"-g,--git,required" help:"Source of git repository."`
	WebsiteUrl         string `arg:"-u,--url,required" help:"Source of the vulnerable website."`
	DisableWeb         bool   `arg:"-w,--web" help:"Disables the website."`
	Port               int    `arg:"-p,--port" default:"8080" help:"Port for the website."`
	EnumerationGitFile string `arg:"-e,--enumeration-file" help:"Enumeration file."`
}

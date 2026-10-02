package views

const layout = `<!DOCTYPE html>
<html><body>{{range .Items}}<p>{{.Name}}</p>{{end}}</body></html>`

func Layout() string { return layout }

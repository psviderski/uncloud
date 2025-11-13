package config

type Context struct {
	Name        string              `yaml:"-"`
	Connections []MachineConnection `yaml:"connections"`
}

func (c *Context) SetDefaultConnection(index int) {
	// Do nothing if the index is out of range.
	if index < 0 || index >= len(c.Connections) {
		return
	}

	selected := c.Connections[index]
	newConnections := make([]MachineConnection, 0, len(c.Connections))
	newConnections = append(newConnections, selected)

	for i, conn := range c.Connections {
		if i != index {
			newConnections = append(newConnections, conn)
		}
	}
	c.Connections = newConnections
}

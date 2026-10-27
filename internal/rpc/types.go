package rpc

type GlobalStat struct {
	DownloadSpeed   string `json:"downloadSpeed"`
	UploadSpeed     string `json:"uploadSpeed"`
	NumActive       string `json:"numActive"`
	NumWaiting      string `json:"numWaiting"`
	NumStopped      string `json:"numStopped"`
	NumStoppedTotal string `json:"numStoppedTotal"`
}

type File struct {
	Index           string   `json:"index"`
	Path            string   `json:"path"`
	Length          string   `json:"length"`
	CompletedLength string   `json:"completedLength"`
	Selected        string   `json:"selected"`
	URIs            []URIRef `json:"uris"`
}

type URIRef struct {
	URI    string `json:"uri"`
	Status string `json:"status"`
}

type BTInfo struct {
	Name string `json:"name"`
}

type BitTorrent struct {
	Info BTInfo `json:"info"`
}

type Task struct {
	Gid             string     `json:"gid"`
	Status          string     `json:"status"`
	TotalLength     string     `json:"totalLength"`
	CompletedLength string     `json:"completedLength"`
	UploadLength    string     `json:"uploadLength"`
	DownloadSpeed   string     `json:"downloadSpeed"`
	UploadSpeed     string     `json:"uploadSpeed"`
	Connections     string     `json:"connections"`
	Dir             string     `json:"dir"`
	Files           []File     `json:"files"`
	BitTorrent      BitTorrent `json:"bittorrent"`
	ErrorCode       string     `json:"errorCode"`
	ErrorMessage    string     `json:"errorMessage"`
	InfoHash        string     `json:"infoHash"`
	NumPieces       string     `json:"numPieces"`
	PieceLength     string     `json:"pieceLength"`
}

func (t Task) Name() string {
	if t.BitTorrent.Info.Name != "" {
		return t.BitTorrent.Info.Name
	}
	if len(t.Files) > 0 && t.Files[0].Path != "" {
		p := t.Files[0].Path
		for i := len(p) - 1; i >= 0; i-- {
			if p[i] == '\\' || p[i] == '/' {
				if i+1 < len(p) {
					return p[i+1:]
				}
				break
			}
		}
		return p
	}
	return t.Gid
}

type VersionInfo struct {
	Version         string   `json:"version"`
	EnabledFeatures []string `json:"enabledFeatures"`
}

package cmd

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	dashboardObjType = "dashboard"
	monitorObjType   = "monitor"

	objRefSep  = "-"
	jsonExt    = ".json"
	touchedExt = ".touched"
)

type objectRef struct {
	OrgID int
	Type  string
	ID    string
}

func objectFilePath(orgID int, objType, objID string) string {
	// orgID/objType-objID.json
	return filepath.Join(strconv.Itoa(orgID), objType+objRefSep+objID+jsonExt)
}

func parseObjectFileName(name string) (string, string, string, error) {
	// objType-objID.json
	ext := filepath.Ext(name)
	if ext != jsonExt && ext != touchedExt {
		return "", "", ext, fmt.Errorf("invalid file extension: %s", ext)
	}

	// objType-objID
	base := strings.TrimSuffix(name, ext)
	split := strings.SplitN(base, objRefSep, 2)
	if len(split) != 2 {
		return "", "", ext, fmt.Errorf("invalid file name: %s", name)
	}

	return split[0], split[1], ext, nil
}

func objectRefFromFile(folder, name string) (objectRef, error) {
	var objRef objectRef

	if orgID, err := strconv.ParseInt(folder, 10, 64); err == nil {
		objRef.OrgID = int(orgID)
	} else {
		return objRef, err
	}

	objType, objID, _, err := parseObjectFileName(name)
	if err != nil {
		return objRef, err
	}

	if objID != "" {
		objRef.ID = objID
	} else {
		return objRef, fmt.Errorf("invalid object ID: %s", objID)
	}

	switch objType {
	case dashboardObjType, monitorObjType:
		objRef.Type = objType
	default:
		return objRef, fmt.Errorf("invalid object type: %s", objType)
	}

	return objRef, nil
}

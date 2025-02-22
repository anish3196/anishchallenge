package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// Location represents a geographical location with both codes and names
type Location struct {
	CityCode     string
	ProvinceCode string
	CountryCode  string
	CityName     string
	ProvinceName string
	CountryName  string
}

// DistributorData represents the data to be persisted
type DistributorData struct {
	Name       string
	ParentName string
	Includes   map[string]bool
	Excludes   map[string]bool
}

// Distributor represents a distribution entity with its permissions
type Distributor struct {
	Name      string
	Parent    *Distributor
	Includes  map[string]bool
	Excludes  map[string]bool
	Locations map[string]*Location // Maps location codes to full location info
}

func NewDistributor(name string, parent *Distributor) *Distributor {
	return &Distributor{
		Name:      name,
		Parent:    parent,
		Includes:  make(map[string]bool),
		Excludes:  make(map[string]bool),
		Locations: make(map[string]*Location),
	}
}

// DistributionSystem manages all distributors
type DistributionSystem struct {
	distributors map[string]*Distributor
	locations    map[string]*Location
}

// NewDistributionSystem creates a new system instance
func NewDistributionSystem() *DistributionSystem {
	return &DistributionSystem{
		distributors: make(map[string]*Distributor),
		locations:    make(map[string]*Location),
	}
}

// LoadLocationData loads geographical data from CSV
func (ds *DistributionSystem) LoadLocationData(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	// Skip header
	_, err = reader.Read()
	if err != nil {
		return err
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if len(record) >= 6 {
			location := &Location{
				CityCode:     record[0],
				ProvinceCode: record[1],
				CountryCode:  record[2],
				CityName:     record[3],
				ProvinceName: record[4],
				CountryName:  record[5],
			}

			cityKey := fmt.Sprintf("%s-%s-%s", location.CityCode, location.ProvinceCode, location.CountryCode)
			provinceKey := fmt.Sprintf("%s-%s", location.ProvinceCode, location.CountryCode)
			countryKey := location.CountryCode

			ds.locations[cityKey] = location
			ds.locations[provinceKey] = location
			ds.locations[countryKey] = location
		}
	}
	return nil
}

// LoadState loads distributor data from the JSON file
func (ds *DistributionSystem) LoadState(filename string) error {
	file, err := os.OpenFile(filename, os.O_RDONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return err
	}

	if stat.Size() == 0 {
		return nil // Empty file, no data to load
	}

	var distributorsData map[string]DistributorData
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&distributorsData); err != nil {
		return err
	}

	// First pass: create all distributors
	for name, data := range distributorsData {
		dist := NewDistributor(name, nil)
		dist.Includes = data.Includes
		dist.Excludes = data.Excludes
		dist.Locations = ds.locations
		ds.distributors[name] = dist
	}

	// Second pass: set up parent relationships
	for name, data := range distributorsData {
		if data.ParentName != "" {
			if parent, exists := ds.distributors[data.ParentName]; exists {
				ds.distributors[name].Parent = parent
			}
		}
	}

	return nil
}

// SaveState saves distributor data to the JSON file
func (ds *DistributionSystem) SaveState(filename string) error {
	distributorsData := make(map[string]DistributorData)

	for name, dist := range ds.distributors {
		var parentName string
		if dist.Parent != nil {
			parentName = dist.Parent.Name
		}

		distributorsData[name] = DistributorData{
			Name:       dist.Name,
			ParentName: parentName,
			Includes:   dist.Includes,
			Excludes:   dist.Excludes,
		}
	}

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "    ")
	return encoder.Encode(distributorsData)
}

func (d *Distributor) AddPermission(permission string, isInclude bool) error {
	if d.Parent != nil {
		// Verify permission is valid with respect to parent
		if !d.Parent.HasPermission(permission) {
			return fmt.Errorf("parent distributor does not have permission for: %s", permission)
		}
	}

	if isInclude {
		d.Includes[permission] = true
	} else {
		d.Excludes[permission] = true
	}
	return nil
}

// HasPermission checks if distribution is allowed in the given region
func (d *Distributor) HasPermission(region string) bool {
	parts := strings.Split(region, "-")

	// Check excludes first
	for excluded := range d.Excludes {
		excludedParts := strings.Split(excluded, "-")
		if isSubregion(parts, excludedParts) {
			return false
		}
	}

	// Check includes
	for included := range d.Includes {
		includedParts := strings.Split(included, "-")
		if isSubregion(parts, includedParts) {
			// Check parent permissions if exists
			if d.Parent != nil {
				return d.Parent.HasPermission(region)
			}
			return true
		}
	}

	return false
}

func isSubregion(region1, region2 []string) bool {
	// If region2 is a country code
	if len(region2) == 1 {
		return region1[len(region1)-1] == region2[0]
	}

	// If region2 is a province-country code
	if len(region2) == 2 {
		return len(region1) >= 2 &&
			region1[len(region1)-2] == region2[0] &&
			region1[len(region1)-1] == region2[1]
	}

	// If region2 is a city-province-country code
	if len(region2) == 3 {
		return len(region1) == 3 &&
			region1[0] == region2[0] &&
			region1[1] == region2[1] &&
			region1[2] == region2[2]
	}

	return false
}

// AddDistributor adds a new distributor to the system
func (ds *DistributionSystem) AddDistributor(name string, parentName string) error {
	if _, exists := ds.distributors[name]; exists {
		return fmt.Errorf("distributor %s already exists", name)
	}

	var parent *Distributor
	if parentName != "" {
		var exists bool
		parent, exists = ds.distributors[parentName]
		if !exists {
			return fmt.Errorf("parent distributor %s does not exist", parentName)
		}
	}

	distributor := NewDistributor(name, parent)
	distributor.Locations = ds.locations
	ds.distributors[name] = distributor
	return nil
}

// AddPermission adds a permission for a distributor
func (ds *DistributionSystem) AddPermission(distributorName, region string, isInclude bool) error {
	distributor, exists := ds.distributors[distributorName]
	if !exists {
		return fmt.Errorf("distributor %s does not exist", distributorName)
	}

	if !ds.ValidateRegion(region) {
		return fmt.Errorf("invalid region code: %s", region)
	}

	return distributor.AddPermission(region, isInclude)
}

// CheckPermission checks if a distributor has permission for a region
func (ds *DistributionSystem) CheckPermission(distributorName, region string) (bool, error) {
	distributor, exists := ds.distributors[distributorName]
	if !exists {
		return false, fmt.Errorf("distributor %s does not exist", distributorName)
	}

	if !ds.ValidateRegion(region) {
		return false, fmt.Errorf("invalid region code: %s", region)
	}

	return distributor.HasPermission(region), nil
}

// ValidateRegion checks if a region code exists
func (ds *DistributionSystem) ValidateRegion(region string) bool {
	_, exists := ds.locations[region]
	return exists
}

// ListDistributors prints all distributors and their permissions
func (ds *DistributionSystem) ListDistributors() {
	fmt.Println("Registered Distributors:")
	for name, dist := range ds.distributors {
		parentName := "none"
		if dist.Parent != nil {
			parentName = dist.Parent.Name
		}
		fmt.Printf("- %s (Parent: %s)\n", name, parentName)
		fmt.Println("  Includes:")
		for region := range dist.Includes {
			fmt.Printf("    - %s\n", region)
		}
		fmt.Println("  Excludes:")
		for region := range dist.Excludes {
			fmt.Printf("    - %s\n", region)
		}
		fmt.Println()
	}
}

func main() {
	// Command line flags
	csvFile := flag.String("csv", "cities.csv", "Path to the locations CSV file")
	dataFile := flag.String("data", "distributors.json", "Path to the distributors data file")
	command := flag.String("cmd", "", "Command to execute (add-distributor, add-permission, check, list)")
	distributorName := flag.String("distributor", "", "Distributor name")
	parentName := flag.String("parent", "", "Parent distributor name (for add-distributor)")
	region := flag.String("region", "", "Region code")
	permissionType := flag.String("type", "include", "Permission type (include/exclude)")

	flag.Parse()

	// Initialize system
	system := NewDistributionSystem()
	err := system.LoadLocationData(*csvFile)
	if err != nil {
		fmt.Printf("Error loading location data: %v\n", err)
		return
	}

	// Load existing distributor data
	err = system.LoadState(*dataFile)
	if err != nil {
		fmt.Printf("Error loading distributor data: %v\n", err)
		return
	}

	var cmdErr error
	switch *command {
	case "list":
		system.ListDistributors()
		return

	case "add-distributor":
		if *distributorName == "" {
			fmt.Println("Error: distributor name is required")
			return
		}
		cmdErr = system.AddDistributor(*distributorName, *parentName)
		if cmdErr == nil {
			fmt.Printf("Successfully added distributor: %s\n", *distributorName)
		}

	case "add-permission":
		if *distributorName == "" || *region == "" {
			fmt.Println("Error: distributor name and region are required")
			return
		}
		isInclude := *permissionType == "include"
		cmdErr = system.AddPermission(*distributorName, *region, isInclude)
		if cmdErr == nil {
			fmt.Printf("Successfully added %s permission for %s to %s\n",
				*permissionType, *region, *distributorName)
		}

	case "check":
		if *distributorName == "" || *region == "" {
			fmt.Println("Error: distributor name and region are required")
			return
		}
		hasPermission, err := system.CheckPermission(*distributorName, *region)
		if err != nil {
			fmt.Printf("Error checking permission: %v\n", err)
			return
		}
		location := system.locations[*region]
		fmt.Printf("Permission check for %s:\n", *distributorName)
		fmt.Printf("Region: %s (%s, %s, %s)\n",
			*region, location.CityName, location.ProvinceName, location.CountryName)
		fmt.Printf("Result: %v\n", hasPermission)

	default:
		fmt.Println("Usage:")
		fmt.Println("1. Add distributor:")
		fmt.Println("   go run main.go -cmd=add-distributor -distributor=DIST1 [-parent=PARENTDIST]")
		fmt.Println("\n2. Add permission:")
		fmt.Println("   go run main.go -cmd=add-permission -distributor=DIST1 -region=REGION-CODE -type=include/exclude")
		fmt.Println("\n3. Check permission:")
		fmt.Println("   go run main.go -cmd=check -distributor=DIST1 -region=REGION-CODE")
		fmt.Println("\n4. List all distributors:")
		fmt.Println("   go run main.go -cmd=list")
	}

	if cmdErr != nil {
		fmt.Printf("Error: %v\n", cmdErr)
		return
	}

	// Save state after successful command execution
	if *command != "check" && *command != "list" {
		if err := system.SaveState(*dataFile); err != nil {
			fmt.Printf("Error saving state: %v\n", err)
		}
	}
}

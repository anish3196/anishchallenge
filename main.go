package main

import (
	"encoding/csv"
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

// ValidateRegion checks if a region code exists in the loaded data
func (d *Distributor) ValidateRegion(region string) bool {
	_, exists := d.Locations[region]
	return exists
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

func main() {
	// Command line flags
	csvFile := flag.String("csv", "cities.csv", "Path to the locations CSV file")
	command := flag.String("cmd", "", "Command to execute (add-distributor, add-permission, check)")
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

	switch *command {
	case "add-distributor":
		if *distributorName == "" {
			fmt.Println("Error: distributor name is required")
			return
		}
		err := system.AddDistributor(*distributorName, *parentName)
		if err != nil {
			fmt.Printf("Error adding distributor: %v\n", err)
			return
		}
		fmt.Printf("Successfully added distributor: %s\n", *distributorName)

	case "add-permission":
		if *distributorName == "" || *region == "" {
			fmt.Println("Error: distributor name and region are required")
			return
		}
		isInclude := *permissionType == "include"
		err := system.AddPermission(*distributorName, *region, isInclude)
		if err != nil {
			fmt.Printf("Error adding permission: %v\n", err)
			return
		}
		fmt.Printf("Successfully added %s permission for %s to %s\n",
			*permissionType, *region, *distributorName)

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
		fmt.Println("   go run main.go -cmd=add-permission -distributor=DIST1 -region=REGION-CODE(City Code-Province Code-Country Code) -type=include/exclude")
		fmt.Println("\n3. Check permission:")
		fmt.Println("   go run main.go -cmd=check -distributor=DIST1 -region=REGION-CODE(City Code-Province Code-Country Code)")
	}
}

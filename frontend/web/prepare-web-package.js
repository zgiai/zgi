const fs = require("fs");
const path = require("path");

// List of dependencies to remove
const removePackages = [
	"nextron",
	"electron",
	"electron-serve",
	"electron-store",
	"electron-builder",
	// Use regex to match all electron-related packages
	/^electron.*/,
];

// Read original package.json (path in Docker build environment)
const packageJson = require("./package.json");

// Create new package.json object
const newPackageJson = {
	...packageJson,
	// Remove electron-related scripts
	scripts: {
		dev: "next dev",
		build: "next build",
		start: "next start",
		"build:web": "next build",
	},
};

// Delete dependencies
const filterDependencies = (deps) => {
	if (!deps) return {};
	return Object.entries(deps).reduce((acc, [key, value]) => {
		const shouldRemove = removePackages.some((pkg) =>
			typeof pkg === "string" ? pkg === key : pkg.test(key),
		);
		if (!shouldRemove) {
			acc[key] = value;
		}
		return acc;
	}, {});
};

// Handle dependencies
newPackageJson.dependencies = filterDependencies(packageJson.dependencies);
newPackageJson.devDependencies = filterDependencies(
	packageJson.devDependencies,
);

// Delete main field and other electron-related fields
delete newPackageJson.main;
delete newPackageJson.postinstall;

// Write new package.json (path in Docker build environment)
fs.writeFileSync(
	"package.web.json",
	JSON.stringify(newPackageJson, null, 2),
	"utf8",
);

console.log("Web package.json has been created successfully");

# uOpTime

Also called μOpTime

Benutze RMAD als Instabilitätsmaß (oder anderes relatives Streuungsmaß in % des Mittelwerts (mean oder median)).

Grenze zur Instabilität: RMAD > 0.01 (1% des Mittels Abweichung)

Probiere unterschiedliche Schätzansätze für CV, RMAD und RCIW

Absolute Minimalkonfiguration:
- IR: nicht anfassen
- SR: 1
- It: 3
- BED: 1s


finde config mit ir 1, verifiziere mit ir 2 und 3



# Setup

Create google cloud Project
Create Service Account
Create Bucket
Create Disk Image with git and golang installed
Copy config file and create own

Its important that the compute engine default service account has the following roles:
- Storage Access (to load the startup script from the bucket)

gp mod

# Run Orchestrator

```
make all
```

```
./build/orchestrator --configFile configFile.toml --credentials /Users/christopher/Uni/MasterThesis/keys/master-thesis-benchmark-d7f8df1edc74.json --benchmark-list-port 5002 --measurement-report-port 5003
```

# Debugging

For debugging the startup script of the VMs, connect to them using ssh and run the following command:
```
sudo journalctl -u google-startup-scripts.service -f
```

Run Startup script:
```
sudo google_metadata_script_runner startup
```
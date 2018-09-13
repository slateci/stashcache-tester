Application to test stashcache instances, use go run stashcache-tester.go or go
build followed by ./stashcache-tester to run.

siteconfig.json is used to specify sites and data sets to be tested.  Format for entries in json file is

[ 
   {
     "dnsname": <hostname of xrootd/stashcache instance,
     "sitename": sitename to use for reporting, e.g. UC_STASH_ORIGIN,
     "hashfile": path to file with sha256 hashes of test files
     "testsetname":  name of test set for reporting
     "testfile": [ "...", "...", ... ] array of file paths for test files
   } ,
  ...
]


There's currently two data sets present:
   MULTIPLE_FILE_TEST:
      /user/sthapa/test-sets/filetest/hashes - hash path
      /user/sthapa/public/test-sets/filetest/test_file.[1-100] data files
   FILE_SIZE_TEST:
      /user/sthapa/test-sets/hashes - hash path
      /user/sthapa/public/test-sets/test.(1|100|1024)M data files
      
  
   

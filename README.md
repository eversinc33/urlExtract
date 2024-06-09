# urlExtract

Recursively extract urls from a web page for reconnaissance. Requires Go>=1.16.

Install using `go install github.com/eversinc33/urlExtract@latest`
```
Usage: linkExtract (flags) [target_url]
  -b string
        Define cookies to be sent with each request using a string like "ID=1ymu32x7;SESSION=29".
  -r int
        Maximum recursion depth. (default 1)
  -s    Log urls that are not based on the target url and thus could out of scope.
```


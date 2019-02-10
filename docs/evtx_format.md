# Outline of the EVTX format.

The format consists of a file header followed by multiple
chunks. Each chunk is 64kb and contains a series of records stored
back to back.

Each record consists of a header followed by binary XML encoded
data. The binary XML can define templates - with special tags
representing insertion points for template arguments.

* Templates have an ID and they should be cached throughout the chunk.

* Newer records may just refer to the template by ID instead of define
  the template again. This means that it is necessary to fully parse
  all records within the chunk even if we are only interested in a
  record towards the end of the chunk - in case it is referring to a
  template defined by an earlier record.

* Strings may be intered throughout the chunk - so a string referred
  to by a later record may just refer to the same string used by an
  earlier record.


## Converting XML to JSON

EVTX logs are in XML but this is hard to work with. It is much easier
to work with JSON and so this library produces a JSON serializable
object.  We need to necessarily transform the XML in some way:

* XML tags are converted to objects.

* XML tag attributes are stored as key value pairs inside their
  containing tag object

* XML CDATA values are stored under the key "Value" in their
  containing tag

* XML tags are stored inside their containing object using the tag
  name as a key.

* If multiple XML tags exist with the same tag name, they are
  converted to a list.

Examples:

```
<EventData></EventData>

"EventData": {}
```

```
<EventData xmlns="https://......"></EventData>

"EventData": {
   "xmlns": "https://...."
}
```

```
<EventData>
   <Data name="foo">foo's value</Data>
</EventData>

"EventData": {
   "Data": {
       "name": "foo",
       "Value": "foo's value"
   }
}
```

```
<EventData>
   <Data name="foo">foo's value</Data>
   <Data name="bar">bar's value</Data>
</EventData>

"EventData": {
   "Data": [
   {
       "name": "foo",
       "Value": "foo's value"
   },
   {
       "name": "bar",
       "Value": "bar's value"
   }
}
```

Additionally the XML is often hard to work with. For example the
constract above has generic <Data> tags with name, value pairs but in
JSON this will be hard to select.

We therefore convert the above pattern into a simpler key/value:

```
<EventData>
   <Data name="foo">foo's value</Data>
   <Data name="bar">bar's value</Data>
</EventData>

"EventData": {
    "foo": "foo's value",
    "bar": "bar's value"
}
```


## Data types

We try to preserve data types as much as possible. So if the XML has
an integer, we emit an integer into the JSON serializable object.

The following are exceptions to make the data more useful:

* Hex encoded XML data types are converted to integers (i.e. if the
  log file says 0x04 we just store the integer 4.

* Timestamps are converted to epoch time floats.
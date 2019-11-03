/*
   Copyright 2018 Velocidex Innovations

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/
package evtx

import (
	"github.com/Velocidex/ordereddict"
)

// EVTX is in XML but this is hard for us to query. So we try to
// normalize some common XML patterns into something which is easier
// to work with.

/* Sometimes the XML looks like:

<EventData>
   <Data name="Thing1"> %Subst% </Data>
   <Data name="Thing2"> %Subst% </Data>
   <Data name="Thing3"> %Subst% </Data>
</Eventdata>

Which converts to json like this:

"Eventdata": {
    "Data": [
       {
         "Name": "thing1",
         "": %Subst%
       },
       {
         "Name": "thing2",
         "": %Subst%
       },
       {
         "Name": "thing3",
         "": %Subst%
       },

    ]
}

But this is really hard to work with. To simplify processing we need
to convert it to:

"EventData": {
   "Thing1": %Subst,
   "Thing2": %Subst,
   "Thing3": %Subst,
}

However we need to keep the order so we can properly interpolate it
into the message template.  We use a very simple OrdereDict for
this. We rely on the MarshalJSON of the ordereddict to ensure the keys
in the JSON are ordered in Event order (otherwise interpolation into
the message will fail).

*/

func NormalizeEventData(expanded interface{}) {
	data, ok := expanded.(*ordereddict.Dict)
	if !ok {
		return
	}
	event_data, pres := data.Get("EventData")
	if !pres {
		return
	}
	event_data_map, ok := event_data.(*ordereddict.Dict)
	if !ok {
		return
	}

	data_tag, pres := event_data_map.Get("Data")
	if !pres {
		return
	}

	data_array, ok := data_tag.([]interface{})
	if !ok {
		return
	}

	result := ordereddict.NewDict()
	for _, item := range data_array {
		item_map, ok := item.(*ordereddict.Dict)
		if !ok {
			return
		}

		// Look for name and "" pairs.
		name_any, pres := item_map.Get("Name")
		if !pres {
			return
		}

		name, ok := name_any.(string)
		if !ok {
			return
		}

		value, pres := item_map.Get("Value")
		if !pres {
			return
		}
		result.Set(name, value)
	}

	data.Set("EventData", result)
}

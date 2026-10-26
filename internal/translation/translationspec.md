# End Goal:
The end goal is to have program which takes html and md files as input, if 
it is html we directly start the translation process if it is md we make the 
md file an html file usign a library.

## Upload:
files are uploaded from the admin page, a post sends the file to the server for processing
## Processing 
md -> Html
Html -> parser -> word stream
-> existance check -> if exists no op ->
if no exists check against library and thesaurus to find words of the same meaning -> if
no similar words make new word and add to db
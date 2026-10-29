No one draws maps. Instead you **tell of places**. A [[runners|letter]] to another Wanax can tell of one named place, such as a [[rivers|river]], a mountain massif, a [[semi-desert|desert]] or a [[cedar-forest|cedar forest]], or of a city your people have seen. When the Runner arrives, the place appears on the reader's map in grey, as [[sight|memory]].

## What a story carries {#telling-of-places-what}

A story is never false, and it carries **only what your own people have seen**. If they saw three hexes of a long river, the story tells of those three hexes and nothing more. It is fixed when the Runner leaves. Anything you see afterwards stays with you.

A city in the story is described **as you last saw it**: its name, its ruler, its walls and its size. If it has changed hands since then, the reader will not know until someone sees it again.

Only a **named** place can be told of, because a place gets its name from whoever reaches it first (see [[naming-the-land]]). You can also tell of any city your people have seen. You cannot pass on what you were only told yourself, because a story has to be first-hand.

## What the reader gets {#telling-of-places-reader}

The told hexes show as grey memory with the note **"Told by Wanax X"**. Told ground opens **no trade, gifts or founding**: a [[trade|trade offer]] needs your own people to have reached the city. You can still send a letter there. If your own people see the ground later, your own memory replaces the story.

A story costs nothing and is not counted. If you want payment for it, ask in the letter. A Runner lost at sea loses its story with it.

## Telling of places {#telling-of-places-how}

In the city panel, choose **Tell of a place** under the letter, then send. In [[keryx|Keryx]]: `keryx message --to Korinth --text "Hear of this" --tell-place "Dark Wood"`. `keryx places` shows which places you can tell of and who has told you of others. <!-- src: server/api/handlers/place_story.go buildStory; server/internal/messenger/story.go deliverStoryTx -->

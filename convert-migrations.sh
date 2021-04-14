#! /bin/bash
rm -rf eyeshade/migrations* eyeshade/seeds
cp -r $BAT_LEDGER/eyeshade/migrations ./eyeshade/ && \
rm eyeshade/migrations/current.js
mkdir -p eyeshade/migrations-2
mkdir -p eyeshade/seeds
for i in $(ls eyeshade/migrations); do
  for j in $(ls eyeshade/migrations/$i/); do
    if [[ -f eyeshade/migrations/$i/$j ]]
    then
      final=eyeshade/migrations-2/$i.$j
      echo $final
      cp eyeshade/migrations/$i/$j eyeshade/migrations-2/$i.$j
    else
      for k in $(ls eyeshade/migrations/$i/$j); do
        o="1"
        if [[ $k == "groups.sql" ]]
        then
          o="0"
        fi
        final=eyeshade/seeds/$i\_$o\_$k
        echo $final
        cp eyeshade/migrations/$i/$j/$k $final
      done
    fi
  done
done
rm -rf eyeshade/migrations
mv eyeshade/migrations-2 eyeshade/migrations

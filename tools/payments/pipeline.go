package payments

type result struct {
	Err error
}

func pipeline(action func(interface{}) error) error{
	// fan out coordinator
	in := chan interface{}
	go coordinator(in, ar.Report...)

	// spin up workers
	out := chan result
	stop := chan struct{}
	defer func (){
		for i:=0; i<submitWorkerCount; i++ {
			stop <-struct{}
		}
		close(stop)
		close(out)
	}()

	for i:=0; i<submitWorkerCount; i++ {
		go worker(action, in, out, stop)
	}

	// wait for workers to complete, if we get an error kill remaining workers (deferred)
	for j:=0; j<len(ar.Report); j++ {
		if result:=<-out; result.Err != nil {
			close(in)
			// return the error
			return result.Err
		}
	}
	close(in)
	return nil
}

func coordinator(fanOut chan interface{}, items ...interface{}) {
	for _, v := range items {
		fanOut <- items
	}
}

func worker(process func(interface{}) error, in chan interface{}, out chan result, stop chan struct{}) {
	for {
		select {
		case m := <-in:
			if err := process(m); err != nil {
				out <- result{Err: err}
			}
			out <- result{}
		case <-stop:
			return
		}
	}
}
